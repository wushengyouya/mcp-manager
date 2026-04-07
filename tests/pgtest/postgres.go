package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/stretchr/testify/require"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const postgresDSNEnv = "MCP_TEST_POSTGRES_DSN"

// HasPostgresTestDSN 返回是否配置了 PostgreSQL 测试 DSN。
func HasPostgresTestDSN() bool {
	return strings.TrimSpace(os.Getenv(postgresDSNEnv)) != ""
}

// NewPostgresDatabaseConfig 创建带隔离 schema 的 PostgreSQL 测试配置。
func NewPostgresDatabaseConfig(t *testing.T) config.DatabaseConfig {
	t.Helper()

	return config.DatabaseConfig{
		Driver:          "postgres",
		DSN:             NewIsolatedPostgresDSN(t),
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Hour,
	}
}

// NewIsolatedPostgresDSN 为当前测试创建隔离 schema，并返回带 search_path 的 DSN。
func NewIsolatedPostgresDSN(t *testing.T) string {
	t.Helper()

	baseDSN := strings.TrimSpace(os.Getenv(postgresDSNEnv))
	if baseDSN == "" {
		t.Skip("未设置 MCP_TEST_POSTGRES_DSN，跳过 PostgreSQL matrix")
	}

	adminDB, err := sql.Open("pgx", baseDSN)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	require.NoError(t, adminDB.PingContext(ctx))
	cancel()

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	_, err = adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, schema))
	cancel()
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = adminDB.ExecContext(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema))
		_ = adminDB.Close()
	})

	return withSearchPath(baseDSN, schema)
}

func withSearchPath(dsn, schema string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err == nil {
			query := parsed.Query()
			query.Set("search_path", schema)
			parsed.RawQuery = query.Encode()
			return parsed.String()
		}
	}

	trimmed := strings.TrimSpace(dsn)
	if strings.Contains(trimmed, "search_path=") {
		return trimmed
	}
	return trimmed + " search_path=" + schema
}
