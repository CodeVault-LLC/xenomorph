package logger

import (
	"os"
	"time"

	"github.com/codevault-llc/xenomorph/internal/shared"
	"github.com/codevault-llc/xenomorph/pkg/embeds"
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

type serverCore struct {
	zapcore.LevelEnabler
	encoder      zapcore.Encoder
	Bot          shared.BotController
	originalCore zapcore.Core
}

func (s *serverCore) With(fields []zapcore.Field) zapcore.Core {
	return &serverCore{
		LevelEnabler: s.LevelEnabler,
		encoder:      s.encoder,
		originalCore: s.originalCore.With(fields),
	}
}

func (s *serverCore) Check(entry zapcore.Entry, checkedEntry *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if s.Enabled(entry.Level) {
		return checkedEntry.AddCore(entry, s)
	}

	return checkedEntry
}

func (s *serverCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	err := s.originalCore.Write(entry, fields)
	if err != nil {
		return err
	}

	// Send log to server
	go func() {
		if entry.Level == zapcore.ErrorLevel || entry.Level == zapcore.WarnLevel {
			embed := embeds.ErrorEmbed(&embeds.ErrorEntry{
				Level: entry.Level.String(),
				Msg:   entry.Message,
				TS:    entry.Time.Format("2006-01-02 15:04:05"),
			})

			if s.Bot == nil {
				return
			}

			if s.Bot.GetChannelFromName("error") == "" {

				return
			}

			err := s.Bot.SendEmbedToChannel(s.Bot.GetChannelFromName("error"), "", &embed)
			if err != nil {
				logger.Error("Failed to send error embed to discord", zap.Error(err))
			}
		}
	}()

	return nil
}

func (s *serverCore) Sync() error {
	return s.originalCore.Sync()
}

func NewLogger() error {
	if _, err := os.Stat("logs"); os.IsNotExist(err) {
		err := os.Mkdir("logs", AppendCode)
		if err != nil {
			return err
		}
	}

	logFile := "logs/" + time.Now().Format("2006-01-02") + ".log"

	fileEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	consoleEncoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    MaxSize,
		MaxBackups: MaxBackups,
		MaxAge:     MaxAge,
	})

	core := zapcore.NewCore(fileEncoder, fileWriter, zapcore.InfoLevel)

	serverLogger := &serverCore{
		LevelEnabler: zapcore.InfoLevel,
		encoder:      zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		originalCore: core,
	}

	log := zap.New(zapcore.NewTee(
		serverLogger,
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), zapcore.DebugLevel), // Console logging
	))

	logger = log

	return nil
}

func AddBot(bot shared.BotController) {
	serverLogger, ok := logger.Core().(*serverCore)
	if !ok {
		return
	}

	serverLogger.Bot = bot
}

func GetLogger() *zap.Logger {
	return logger
}
