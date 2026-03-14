package logger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/stretchr/testify/require"
)

// TestInit_Console 验证 console 格式初始化成功。
func TestInit_Console(t *testing.T) {
	err := Init(config.LogConfig{
		Level:  "debug",
		Format: "console",
		Output: "stdout",
	})
	require.NoError(t, err)
}

// TestInit_JSON 验证 json 格式初始化成功。
func TestInit_JSON(t *testing.T) {
	err := Init(config.LogConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	})
	require.NoError(t, err)
}

// TestInit_InvalidLevel 验证无效日志级别返回错误。
func TestInit_InvalidLevel(t *testing.T) {
	err := Init(config.LogConfig{
		Level:  "invalid",
		Format: "console",
		Output: "stdout",
	})
	require.Error(t, err)
}

// TestL_BeforeInit 验证初始化前 L() 返回非 nil 的 nop logger。
func TestL_BeforeInit(t *testing.T) {
	// 重置全局状态
	mu.Lock()
	global = nil
	sugar = nil
	mu.Unlock()

	l := L()
	require.NotNil(t, l)
}

// TestS_BeforeInit 验证初始化前 S() 返回非 nil 的 nop sugar logger。
func TestS_BeforeInit(t *testing.T) {
	mu.Lock()
	global = nil
	sugar = nil
	mu.Unlock()

	s := S()
	require.NotNil(t, s)
}

// TestInit_FileOutput 验证文件输出时日志文件被创建。
func TestInit_FileOutput(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	err := Init(config.LogConfig{
		Level:      "info",
		Format:     "json",
		Output:     logFile,
		MaxSize:    1,
		MaxBackups: 1,
		MaxAge:     1,
	})
	require.NoError(t, err)

	// 写一条日志以触发文件创建
	L().Info("test log entry")
	_ = L().Sync()

	_, err = os.Stat(logFile)
	require.NoError(t, err, "日志文件应已被创建")
}
