package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Logger struct {
	level int
	mu    sync.Mutex
	std   *log.Logger
}

const (
	levelDebug = iota
	levelInfo
	levelWarn
	levelError
)

func NewLogger(levelRaw string, logType string) *Logger {
	level := levelInfo
	switch strings.ToUpper(levelRaw) {
	case "DEBUG", "VERBOSE":
		level = levelDebug
	case "NOTICE", "INFO":
		level = levelInfo
	case "WARN", "WARNING":
		level = levelWarn
	case "ERROR":
		level = levelError
	}

	_ = logType

	return &Logger{level: level, std: log.New(os.Stdout, "", 0)}
}

func (l *Logger) logf(level int, label string, format string, args ...any) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.std.Printf("%s %s | %s", time.Now().Format(time.RFC3339), label, fmt.Sprintf(format, args...))
}

func (l *Logger) Debugf(format string, args ...any) { l.logf(levelDebug, "DEBUG", format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.logf(levelInfo, "INFO", format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.logf(levelWarn, "WARN", format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.logf(levelError, "ERROR", format, args...) }
