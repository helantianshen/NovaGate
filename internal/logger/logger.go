package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log 全局的高性能日志实例
var Log *zap.Logger

// Init 初始化 Zap
// level: "debug", "info", "warn", "error"
func Init(level string) {
	// 设置日志级别 (生产环境压测时建议设为 warn 或 error)
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel // 解析失败默认 Info
	}

	// 核心配置：使用适合生产环境的 JSON 编码器
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder   // 友好的时间格式
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder // 级别大写，如 INFO

	// 构建 Core (决定日志写到哪里、怎么编码、什么级别)
	// 这里先写到终端 (os.Stdout)，后续可以配合 lumberjack 写到文件并自动轮转
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.Lock(os.Stdout),
		zapLevel,
	)

	// 生成 Logger，开启 Caller 记录行号
	Log = zap.New(core, zap.AddCaller())
}

// Sync 程序退出前，将缓冲区日志刷入磁盘
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
