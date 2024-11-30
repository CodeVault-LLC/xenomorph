package logger

import (
	"fmt"
	"os"
	"time"

	"github.com/codevault-llc/xenomorph/internal/shared"
	"github.com/codevault-llc/xenomorph/pkg/embeds"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Log is the global logger instance
var Log *zap.Logger

// serverCore sends logs to a remote server
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
				Ts:    entry.Time.Format("2006-01-02 15:04:05"),
			})

			if s.Bot == nil {
				return
			}

			if s.Bot.GetChannelFromName("error") == "" {
				fmt.Println("Failed to get error channel ID")
				return
			}

			err := s.Bot.SendEmbedToChannel(s.Bot.GetChannelFromName("error"), "", &embed)
			if err != nil {
				Log.Error("Failed to send error embed to channel", zap.Error(err))
			}
		}
	}()
	return nil
}

func (s *serverCore) Sync() error {
	return s.originalCore.Sync()
}

// InitLogger initializes the logger with a custom core
func InitLogger(bot shared.BotController) (*zap.Logger, error) {
	if _, err := os.Stat("logs"); os.IsNotExist(err) {
		err := os.Mkdir("logs", 0o755)
		if err != nil {
			return nil, err
		}
	}

	logFile := "logs/" + time.Now().Format("2006-01-02") + ".log"

	fileEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	consoleEncoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
	})

	core := zapcore.NewCore(fileEncoder, fileWriter, zapcore.InfoLevel)

	serverLogger := &serverCore{
		LevelEnabler: zapcore.InfoLevel,
		encoder:      zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		Bot:          bot,
		originalCore: core,
	}

	logger := zap.New(zapcore.NewTee(
		serverLogger,
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), zapcore.DebugLevel), // Console logging
	))

	Log = logger
	return logger, nil
}
