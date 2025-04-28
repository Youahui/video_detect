package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"strings"
)

func NewLogWithSplitting(sp LogSplitting, loglevel string) *zap.Logger {
	// lower case loglevel
	loglevel = strings.ToLower(loglevel)

	var level zapcore.Level
	switch loglevel {
	case "debug":
		level = zap.DebugLevel
	case "info":
		level = zap.InfoLevel
	case "error":
		level = zap.ErrorLevel
	default:
		level = zap.InfoLevel
	}
	atomicLevel := zap.NewAtomicLevel()
	atomicLevel.SetLevel(level)
	core := zapcore.NewCore(
		getEncoder(),
		setLogWriter(sp),
		level)

	// 开启开发模式，堆栈跟踪
	caller := zap.AddCaller()
	// 开启文件及行号
	development := zap.Development()
	// 设置初始化字段,如：添加一个服务器名称
	// filed := zap.Fields(zap.String("serviceName", "serviceName"))
	// 构造日志
	return zap.New(core, caller, development)
}

func getEncoder() zapcore.Encoder {
	return zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "linenum",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,  // 小写编码器
		EncodeTime:     zapcore.ISO8601TimeEncoder,     // ISO8601 UTC 时间格式
		EncodeDuration: zapcore.SecondsDurationEncoder, //
		EncodeCaller:   zapcore.FullCallerEncoder,      // 全路径编码器
		EncodeName:     zapcore.FullNameEncoder,
	})
}

/*
LogSplitting 日志分割

	FileName   文件名（路径）
	MaxSize    日志最大存储大小（MB）
	MaxAge     日志保存天数
	MaxBackups 日志最大存储数量（会被MaxAge删除）
	LocalTime  是否使用本地时间，默认使用UTC
	Compress   是否使用GZIP格式压缩，默认不压缩
*/
type LogSplitting struct {
	FileName   string `json:"fileName"`
	MaxSize    int    `json:"maxSize"`
	MaxAge     int    `json:"maxAge"`
	MaxBackups int    `json:"maxBackups"`
	LocalTime  bool   `json:"localTime"`
	Compress   bool   `json:"compress"`
}

// 日志分割函数

func setLogWriter(sp LogSplitting) zapcore.WriteSyncer {
	return zapcore.AddSync(&lumberjack.Logger{
		Filename:   sp.FileName,
		MaxSize:    sp.MaxSize,
		MaxAge:     sp.MaxAge,
		MaxBackups: sp.MaxBackups,
		LocalTime:  sp.LocalTime,
		Compress:   sp.Compress,
	})
}
