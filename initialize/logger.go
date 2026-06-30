package initialize

import (
	"os"
	"path/filepath"
	"strings"

	"admin/global"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// InitLogger 初始化全局 Zap Logger，并同时输出到控制台和轮转日志文件。
func InitLogger(conf *Config) {
	loggerConf := normalizeLoggerConfig(conf.Logger)

	if err := os.MkdirAll(filepath.Dir(loggerConf.Output), 0755); err != nil {
		panic("create log directory failed: " + err.Error())
	}

	core := zapcore.NewTee(
		zapcore.NewCore(getLoggerEncoder(loggerConf.Format), zapcore.AddSync(os.Stdout), getLoggerLevel(loggerConf.Level)),
		zapcore.NewCore(getLoggerEncoder(loggerConf.Format), zapcore.AddSync(getLoggerWriter(loggerConf)), getLoggerLevel(loggerConf.Level)),
	)

	global.Logger = zap.New(
		core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)

	global.Logger.Info("logger initialized",
		zap.String("level", loggerConf.Level),
		zap.String("format", loggerConf.Format),
		zap.String("output", loggerConf.Output),
	)
}

// normalizeLoggerConfig 为日志配置填充默认值，避免配置缺省导致初始化失败。
func normalizeLoggerConfig(conf LoggerConfig) LoggerConfig {
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

// getLoggerLevel 将配置中的字符串日志级别转换为 zapcore.Level。
func getLoggerLevel(level string) zapcore.Level {
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

// getLoggerEncoder 根据配置选择 JSON 或控制台格式的日志编码器。
func getLoggerEncoder(format string) zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeDuration = zapcore.StringDurationEncoder

	if strings.ToLower(format) == "json" {
		return zapcore.NewJSONEncoder(encoderConfig)
	}

	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	return zapcore.NewConsoleEncoder(encoderConfig)
}

// getLoggerWriter 创建带 lumberjack 轮转能力的日志文件输出。
func getLoggerWriter(conf LoggerConfig) zapcore.WriteSyncer {
	return zapcore.AddSync(&lumberjack.Logger{
		Filename:   conf.Output,
		MaxSize:    conf.MaxSize,
		MaxBackups: conf.MaxBackups,
		MaxAge:     conf.MaxAge,
		Compress:   conf.Compress,
	})
}
