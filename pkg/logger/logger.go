package logger

import (
	"fmt"
	"os"
	"sync"

	"github.com/mikasa/mcp-manager/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	global *zap.Logger
	sugar  *zap.SugaredLogger
	mu     sync.RWMutex
)

// Init 初始化全局日志器。
func Init(cfg config.LogConfig) error {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return err
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "time"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder

	var encoder zapcore.Encoder
	if cfg.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	} else {
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	}

	writer := zapcore.AddSync(os.Stdout)
	if cfg.Output != "" && cfg.Output != "stdout" {
		writer = zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.Output,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		})
	}

	l := zap.New(zapcore.NewCore(encoder, writer, level), zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	mu.Lock()
	defer mu.Unlock()
	global = l
	sugar = l.Sugar()
	return nil
}

func parseLevel(level string) (zapcore.Level, error) {
	switch level {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("未知日志级别: %s", level)
	}
}

// L 返回原生日志器。
func L() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if global == nil {
		global = zap.NewNop()
		sugar = global.Sugar()
	}
	return global
}

// S 返回 SugaredLogger。
func S() *zap.SugaredLogger {
	mu.RLock()
	defer mu.RUnlock()
	if sugar == nil {
		global = zap.NewNop()
		sugar = global.Sugar()
	}
	return sugar
}
