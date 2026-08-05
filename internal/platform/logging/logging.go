package logging

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	platformconfig "admin/internal/platform/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Logger struct {
	*zap.Logger
	output *lumberjack.Logger
}

func New(conf platformconfig.LoggerConfig) (*Logger, error) {
	conf = normalize(conf)
	if err := os.MkdirAll(filepath.Dir(conf.Output), 0o755); err != nil {
		return nil, err
	}

	output := &lumberjack.Logger{
		Filename:   conf.Output,
		MaxSize:    conf.MaxSize,
		MaxBackups: conf.MaxBackups,
		MaxAge:     conf.MaxAge,
		Compress:   conf.Compress,
	}
	level := loggerLevel(conf.Level)
	core := zapcore.NewTee(
		zapcore.NewCore(loggerEncoder(conf.Format), noSyncWriter{File: os.Stdout}, level),
		zapcore.NewCore(loggerEncoder(conf.Format), zapcore.AddSync(output), level),
	)
	return &Logger{
		Logger: zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)),
		output: output,
	}, nil
}

func (logger *Logger) Close() error {
	return errors.Join(logger.Sync(), logger.output.Close())
}

func normalize(conf platformconfig.LoggerConfig) platformconfig.LoggerConfig {
	if conf.Level == "" {
		conf.Level = "debug"
	}
	if conf.Format == "" {
		conf.Format = "console"
	}
	if conf.Output == "" {
		conf.Output = "logs/app.log"
	}
	if conf.MaxSize <= 0 {
		conf.MaxSize = 100
	}
	if conf.MaxBackups <= 0 {
		conf.MaxBackups = 7
	}
	if conf.MaxAge <= 0 {
		conf.MaxAge = 30
	}
	return conf
}

type noSyncWriter struct {
	*os.File
}

func (noSyncWriter) Sync() error { return nil }

func loggerLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func loggerEncoder(format string) zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	if strings.EqualFold(format, "json") {
		return zapcore.NewJSONEncoder(encoderConfig)
	}
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	return zapcore.NewConsoleEncoder(encoderConfig)
}
