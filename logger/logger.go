package logger

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"nofx/config"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	// Log 全局logger实例
	Log *logrus.Logger

	// telegramHook 保存hook引用，用于优雅关闭
	telegramHook *TelegramHook

	// file 对日志文件的引用，用于优雅关闭
	logFile *os.File

	// stdLogWriter 用于捕获标准日志输出的writer
	stdLogWriter *LogWriter
)

// LogWriter 用于捕获标准日志输出的自定义writer
type LogWriter struct {
	writer io.Writer
	mutex  sync.Mutex
}

// Write 实现io.Writer接口
func (lw *LogWriter) Write(p []byte) (n int, err error) {
	lw.mutex.Lock()
	defer lw.mutex.Unlock()

	// 将标准日志内容写入我们的日志系统
	message := string(p)
	// 去除换行符
	if len(message) > 0 && message[len(message)-1] == '\n' {
		message = message[:len(message)-1]
	}

	// 写入到我们自定义的日志系统
	if Log != nil {
		Log.Info(message)
	}

	return len(p), nil
}

// PlainFormatter 自定义格式化器，用于输出简洁的日志格式
type PlainFormatter struct{}

// Format 实现logrus.Formatter接口
func (f *PlainFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}

	// 获取级别并转为大写
	level := strings.ToUpper(entry.Level.String())
	
	// 根据级别添加emoji前缀
	var prefix string
	switch level {
	case "INFO":
		prefix = "INFO"
	case "WARN":
		prefix = "WARN"
	case "ERROR":
		prefix = "ERROR"
	case "DEBUG":
		prefix = "DEBUG"
	case "FATAL":
		prefix = "FATAL"
	case "PANIC":
		prefix = "PANIC"
	default:
		prefix = level
	}

	// 写入格式化的日志
	b.WriteString(prefix)
	b.WriteString(" ")
	b.WriteString(entry.Message)
	b.WriteString("\n")

	return b.Bytes(), nil
}

// ============================================================================
// 初始化函数
// ============================================================================

// Init 初始化全局logger
// 如果config为nil，使用默认配置（console输出，info级别）
func Init(cfg *Config) error {
	Log = logrus.New()

	// 如果没有配置，使用默认值
	if cfg == nil {
		cfg = &Config{Level: "info"}
	}

	// 设置默认值
	cfg.SetDefaults()

	// 设置日志级别
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	Log.SetLevel(level)

	// 创建日志目录
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %v", err)
	}

	// 创建日志文件
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	logFilename := filepath.Join(logDir, timestamp+".log")
	file, err := os.OpenFile(logFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("创建日志文件失败: %v", err)
	}
	logFile = file

	// 创建用于捕获标准日志的writer
	stdLogWriter = &LogWriter{
		writer: file,
	}

	// 设置日志同时输出到控制台和文件
	multiWriter := io.MultiWriter(os.Stdout, file)
	Log.SetOutput(multiWriter)

	// 设置自定义格式化器
	Log.SetFormatter(&PlainFormatter{})

	// 添加Telegram Hook（可选）
	if cfg.Telegram != nil && cfg.Telegram.Enabled {
		if err := setupTelegramHook(cfg.Telegram); err != nil {
			Log.Warnf("初始化Telegram推送失败，将继续使用普通日志: %v", err)
		}
	}

	return nil
}

// CaptureStdLog 将标准日志库的输出重定向到我们的日志系统
func CaptureStdLog() {
	if stdLogWriter != nil {
		// 重定向标准日志输出到我们的writer
		log.SetOutput(stdLogWriter)
	}
}

// FileHook 是一个日志钩子，用于将日志写入文件
type FileHook struct {
	Writer    *os.File
	Formatter logrus.Formatter
}

// Fire 实现logrus.Hook接口
func (hook *FileHook) Fire(entry *logrus.Entry) error {
	line, err := hook.Formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = hook.Writer.Write(line)
	return err
}

// Levels 实现logrus.Hook接口
func (hook *FileHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// setupTelegramHook 设置Telegram Hook
func setupTelegramHook(telegramCfg *TelegramConfig) error {
	hook, err := NewTelegramHook(telegramCfg)
	if err != nil {
		return err
	}

	Log.AddHook(hook)
	telegramHook = hook
	Log.Info("✅ Telegram日志推送已启用")
	return nil
}

// InitWithSimpleConfig 使用简化配置初始化logger
// 适用于只需要基本功能的场景
func InitWithSimpleConfig(level string) error {
	return Init(&Config{Level: level})
}

// InitWithTelegram 使用Telegram配置初始化logger
func InitWithTelegram(botToken string, chatID int64) error {
	return Init(&Config{
		Level: "info",
		Telegram: &TelegramConfig{
			Enabled:  true,
			BotToken: botToken,
			ChatID:   chatID,
		},
	})
}

// InitFromLogConfig 从config.LogConfig初始化logger
func InitFromLogConfig(logConfig *config.LogConfig) error {
	if logConfig == nil {
		return InitWithSimpleConfig("info")
	}

	cfg := &Config{
		Level: logConfig.Level,
	}

	if cfg.Level == "" {
		cfg.Level = "info"
	}

	// 如果启用了Telegram，添加配置
	if logConfig.Telegram != nil && logConfig.Telegram.Enabled {
		if botToken := logConfig.Telegram.BotToken; botToken != "" && logConfig.Telegram.ChatID != 0 {
			cfg.Telegram = &TelegramConfig{
				Enabled:  true,
				BotToken: botToken,
				ChatID:   logConfig.Telegram.ChatID,
				MinLevel: logConfig.Telegram.MinLevel,
			}
		}
	}

	return Init(cfg)
}

// InitFromParams 从参数初始化logger
// 适用于不依赖config包的场景
func InitFromParams(level string, telegramEnabled bool, botToken string, chatID int64) error {
	cfg := &Config{Level: level}

	if telegramEnabled && botToken != "" && chatID != 0 {
		cfg.Telegram = &TelegramConfig{
			Enabled:  true,
			BotToken: botToken,
			ChatID:   chatID,
		}
	}

	return Init(cfg)
}

// Shutdown 优雅关闭logger（主要用于关闭Telegram发送器和日志文件）
func Shutdown() {
	if telegramHook != nil {
		telegramHook.Stop()
		telegramHook = nil
	}

	if logFile != nil {
		logFile.Close()
		logFile = nil
	}

	if stdLogWriter != nil {
		stdLogWriter.writer = nil
	}
}

// ============================================================================
// 日志记录函数
// ============================================================================

// WithFields 创建带字段的logger entry
func WithFields(fields logrus.Fields) *logrus.Entry {
	return Log.WithFields(fields)
}

// WithField 创建带单个字段的logger entry
func WithField(key string, value interface{}) *logrus.Entry {
	return Log.WithField(key, value)
}

// add debug, info, warn
func Debug(args ...interface{}) {
	Log.Debug(args...)
}

func Info(args ...interface{}) {
	Log.Info(args...)
}

func Warn(args ...interface{}) {
	Log.Warn(args...)
}

func Debugf(format string, args ...interface{}) {
	Log.Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	Log.Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	Log.Warnf(format, args...)
}

func Error(args ...interface{}) {
	Log.Error(args...)
}

func Errorf(format string, args ...interface{}) {
	Log.Errorf(format, args...)
}

func Fatal(args ...interface{}) {
	Log.Fatal(args...)
}

func Fatalf(format string, args ...interface{}) {
	Log.Fatalf(format, args...)
}

func Panic(args ...interface{}) {
	Log.Panic(args...)
}

func Panicf(format string, args ...interface{}) {
	Log.Panicf(format, args...)
}