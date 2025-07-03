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

type clientCore struct {
	zapcore.LevelEnabler
	encoder      zapcore.Encoder
	originalCore zapcore.Core
}

func (c *clientCore) With(fields []zapcore.Field) zapcore.Core {
	return &clientCore{
		LevelEnabler: c.LevelEnabler,
		encoder:      c.encoder,
		originalCore: c.originalCore.With(fields),
	}
}

func (c *clientCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

func (c *clientCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	return c.originalCore.Write(entry, fields)
}

func (c *clientCore) Sync() error {
	return c.originalCore.Sync()
}

func InitClientLogger(mode string) error {
	var cores []zapcore.Core

	if mode == "production" {
		serverEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
		serverWriter := zapcore.AddSync(os.Stderr)

		core := &clientCore{
			LevelEnabler: zapcore.InfoLevel,
			encoder:      serverEncoder,
			originalCore: zapcore.NewCore(serverEncoder, serverWriter, zapcore.InfoLevel),
		}
		cores = append(cores, core)
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

	log := zap.New(zapcore.NewTee(cores...))
	Init(log)
	return nil
}
