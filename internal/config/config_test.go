package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("MCP_SERVER_PORT", "9999")
	t.Setenv("MCP_JWT_SECRET", "test-secret")

	cfg, err := Load("/tmp/definitely-not-exists")
	require.NoError(t, err)
	require.Equal(t, 9999, cfg.Server.Port)
	require.Equal(t, "test-secret", cfg.JWT.Secret)
	require.Equal(t, "sqlite", cfg.Database.Driver)
}

func TestLoad_Validate(t *testing.T) {
	old := os.Getenv("MCP_SERVER_PORT")
	t.Cleanup(func() {
		_ = os.Setenv("MCP_SERVER_PORT", old)
	})
	_ = os.Setenv("MCP_SERVER_PORT", "70000")
	_, err := Load("/tmp/definitely-not-exists")
	require.Error(t, err)
}
