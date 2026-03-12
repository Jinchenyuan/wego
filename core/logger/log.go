package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Level int8

const (
	Debug Level = iota
	Info
	Warn
	Error
	Fatal
)

func (level Level) Key() string {
	return "level"
}

func (level Level) String() string {
	switch level {
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	case Fatal:
		return "FATAL"
	default:
		return ""
	}
}

func ParseLevel(str string) Level {
	switch strings.ToUpper(str) {
	case "DEBUG":
		return Debug
	case "INFO":
		return Info
	case "WARN":
		return Warn
	case "ERROR":
		return Error
	case "FATAL":
		return Fatal
	default:
		return Info
	}
}

var defaultLogger = log.New(os.Stdout, "", 0)

var exitProcess = os.Exit

var loggerRegistry sync.Map

const (
	ansiReset = "\033[0m"
)

type Logger struct {
	mu          sync.RWMutex
	serviceName string
	level       Level
	base        *log.Logger
}

// NewLogger creates an independent logger instance for a service.
func NewLogger(serviceName string) *Logger {
	name := strings.TrimSpace(serviceName)
	if name == "" {
		name = "default"
	}

	return &Logger{
		serviceName: name,
		level:       Info,
		base:        defaultLogger,
	}
}

// GetLogger returns a shared logger instance by service name.
func GetLogger(serviceName string) *Logger {
	name := strings.TrimSpace(serviceName)
	if name == "" {
		name = "default"
	}

	v, ok := loggerRegistry.Load(name)
	if ok {
		return v.(*Logger)
	}

	created := NewLogger(name)
	real, _ := loggerRegistry.LoadOrStore(name, created)
	return real.(*Logger)
}

func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *Logger) Level() Level {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.level
}

func (l *Logger) Debug(v ...any) {
	l.log(Debug, v...)
}

func (l *Logger) Info(v ...any) {
	l.log(Info, v...)
}

func (l *Logger) Warn(v ...any) {
	l.log(Warn, v...)
}

func (l *Logger) Error(v ...any) {
	l.log(Error, v...)
}

func (l *Logger) Fatal(v ...any) {
	l.log(Fatal, v...)
	exitProcess(1)
}

func (l *Logger) log(level Level, v ...any) {
	l.mu.RLock()
	currentLevel := l.level
	serviceName := l.serviceName
	base := l.base
	l.mu.RUnlock()

	if level < currentLevel {
		return
	}

	ts := time.Now().Format("2006-01-02 15:04:05")
	prefix := fmt.Sprintf("[%s] [%s %s]:", ts, serviceName, level.String())
	if color := levelColor(level); color != "" {
		prefix = color + prefix + ansiReset
	}
	base.Println(append([]any{prefix}, v...)...)
}

func levelColor(level Level) string {
	switch level {
	case Debug:
		return "\033[36m"
	case Info:
		return "\033[32m"
	case Warn:
		return "\033[33m"
	case Error:
		return "\033[31m"
	case Fatal:
		return "\033[1;31m"
	default:
		return ""
	}
}
