package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	AppendCode = 0o755

	MaxSize    = 10
	MaxBackups = 3
	MaxAge     = 28
)

var logger *zap.Logger

// serverCore sends logs to your remote server (e.g., Discord)
type serverCore struct {
	zapcore.LevelEnabler
	encoder      zapcore.Encoder
	originalCore zapcore.Core
}

func (s *serverCore) With(fields []zapcore.Field) zapcore.Core {
	return &serverCore{
		LevelEnabler: s.LevelEnabler,
		encoder:      s.encoder,
		originalCore: s.originalCore.With(fields),
	}
}

func (s *serverCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if s.Enabled(entry.Level) {
		return ce.AddCore(entry, s)
	}
	return ce
}

func (s *serverCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	err := s.originalCore.Write(entry, fields)
	if err != nil {
		return err
	}

	// Optional async remote send
	/*go func() {
		if ServerAvailable() && (entry.Level >= zapcore.WarnLevel) {
			// Send to your server
			// Replace this stub with real logic (e.g., Discord, API, etc.)
			_ = SendLogToServer(entry)
		}
	}()*/

	return nil
}

func (s *serverCore) Sync() error {
	return s.originalCore.Sync()
}

// NewLogger creates a zap logger based on environment
func NewLogger(mode string) error {
	var cores []zapcore.Core

	// Production-only server logging
	if mode == "production" {
		serverEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
		serverWriter := zapcore.AddSync(os.Stderr) // Replace with a discard writer if needed

		serverCore := &serverCore{
			LevelEnabler: zapcore.InfoLevel,
			encoder:      serverEncoder,
			originalCore: zapcore.NewCore(serverEncoder, serverWriter, zapcore.InfoLevel),
		}
		cores = append(cores, serverCore)
	}


	if mode == "development" {
		path, err := os.UserConfigDir()
		if err != nil {
			return err
		}

		logDir := path + "/xenomorph/logs"
		
		if err := os.MkdirAll(logDir, AppendCode); err != nil {
			return err
		}

		if err := os.Chmod(logDir, AppendCode); err != nil {
			return err
		}

		logFile := logDir + "/xenomorph.log"

		fileEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
		fileWriter := zapcore.AddSync(&lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    MaxSize,
			MaxBackups: MaxBackups,
			MaxAge:     MaxAge,
		})

		consoleEncoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())

		cores = append(cores,
			zapcore.NewCore(fileEncoder, fileWriter, zapcore.InfoLevel),
			zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), zapcore.DebugLevel),
		)
	}

	logger = zap.New(zapcore.NewTee(cores...))
	return nil
}

func GetLogger() *zap.Logger {
	return logger
}
