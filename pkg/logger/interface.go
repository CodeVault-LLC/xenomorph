package logger

import "go.uber.org/zap"

var log *zap.Logger

// Init sets the global logger instance
func Init(l *zap.Logger) {
	log = l
}

// L returns the global logger
func L() *zap.Logger {
	if log == nil {
		panic("logger not initialized")
	}
	return log
}
