package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/mikasa/mcp-manager/tests/pgtest"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type healthCheckerStub struct{}

func (healthCheckerStub) Start() {}
func (healthCheckerStub) Stop()  {}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            18080,
			ReadTimeout:     time.Second,
			WriteTimeout:    time.Second,
			ShutdownTimeout: time.Second,
		},
		Database: config.DatabaseConfig{
			Driver:          "sqlite",
			DSN:             filepath.Join(t.TempDir(), "bootstrap.db"),
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: time.Hour,
		},
		JWT: config.JWTConfig{
			Issuer:     "test",
			Secret:     "test-secret",
			AccessTTL:  time.Hour,
			RefreshTTL: 24 * time.Hour,
		},
		HealthCheck: config.HealthCheckConfig{
			Enabled:          false,
			Interval:         time.Second,
			Timeout:          time.Second,
			FailureThreshold: 1,
		},
		Audit: config.AuditConfig{
			RetentionDays:   7,
			CleanupInterval: time.Hour,
		},
		App: config.AppConfig{
			Name:              "mcp-manager",
			Version:           "test",
			InitAdminUsername: "root",
			InitAdminPassword: "admin123456",
			InitAdminEmail:    "root@example.com",
		},
		History: config.HistoryConfig{
			MaxBodyBytes: 8192,
			Compression:  "none",
		},
		Log: config.LogConfig{Level: "info", Format: "console", Output: "stdout"},
	}
}

func TestBuilderBuildConstructsServerAndAllowsRuntimeFactoryOverride(t *testing.T) {
	require.NoError(t, logger.Init(config.LogConfig{Level: "info", Format: "console", Output: "stdout"}))

	runBootstrapMatrix(t, func(t *testing.T, cfg config.Config) {
		called := false
		app, err := NewBuilder(cfg).
			WithRuntimeFactory(func(appCfg config.AppConfig) *mcpclient.Manager {
				called = true
				return mcpclient.NewManager(appCfg)
			}).
			Build()
		require.NoError(t, err)
		require.True(t, called)
		require.NotNil(t, app.Server)
		require.NotNil(t, app.Cleanup)
		require.Equal(t, "127.0.0.1:18080", app.Server.Addr)

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		app.Server.Handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		app.Cleanup()
	})
}

func TestBuilderReconcilesStatusesBeforeHealthCheckerFactoryRuns(t *testing.T) {
	require.NoError(t, logger.Init(config.LogConfig{Level: "info", Format: "console", Output: "stdout"}))

	runBootstrapMatrix(t, func(t *testing.T, cfg config.Config) {
		cfg.HealthCheck.Enabled = true

		db, err := database.Init(cfg.Database)
		require.NoError(t, err)
		require.NoError(t, database.Migrate(db))
		serviceRepo := repository.NewMCPServiceRepository(db)
		require.NoError(t, serviceRepo.Create(context.Background(), &entity.MCPService{
			Name:          "stale-connected",
			TransportType: entity.TransportTypeStreamableHTTP,
			URL:           "http://stale.test/mcp",
			Status:        entity.ServiceStatusConnected,
		}))
		require.NoError(t, database.Close())

		factoryObservedReconciled := false
		app, err := NewBuilder(cfg).
			WithHealthCheckerFactory(func(_ *mcpclient.Manager, _ config.HealthCheckConfig, _ func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error) HealthChecker {
				checkDB := openInspectDB(t, cfg.Database)
				sqlDB, dbErr := checkDB.DB()
				require.NoError(t, dbErr)
				defer sqlDB.Close()

				checkRepo := repository.NewMCPServiceRepository(checkDB)
				item, getErr := checkRepo.GetByName(context.Background(), "stale-connected")
				require.NoError(t, getErr)
				factoryObservedReconciled = item.Status == entity.ServiceStatusDisconnected
				return healthCheckerStub{}
			}).
			Build()
		require.NoError(t, err)
		require.True(t, factoryObservedReconciled)
		app.Cleanup()
	})
}

func runBootstrapMatrix(t *testing.T, fn func(t *testing.T, cfg config.Config)) {
	t.Helper()

	t.Run("sqlite", func(t *testing.T) {
		fn(t, testConfig(t))
	})

	t.Run("postgres", func(t *testing.T) {
		cfg := testConfig(t)
		cfg.Database = pgtest.NewPostgresDatabaseConfig(t)
		fn(t, cfg)
	})
}

func openInspectDB(t *testing.T, cfg config.DatabaseConfig) *gorm.DB {
	t.Helper()

	switch cfg.Driver {
	case "sqlite":
		db, err := gorm.Open(sqlite.Open(cfg.DSN), &gorm.Config{})
		require.NoError(t, err)
		return db
	case "postgres":
		db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
		require.NoError(t, err)
		return db
	default:
		t.Fatalf("unsupported inspect driver: %s", cfg.Driver)
		return nil
	}
}
