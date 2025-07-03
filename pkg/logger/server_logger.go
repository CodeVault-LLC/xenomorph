package logger

import (
	"os"
	"time"

	"github.com/codevault-llc/xenomorph/internal/bot/embeds"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var notifier BotNotifier

func SetBotNotifier(n BotNotifier) {
	notifier = n
}

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
	if err := s.originalCore.Write(entry, fields); err != nil {
		return err
	}

	// Async send error/warning to Discord
	go func() {
		if entry.Level == zapcore.ErrorLevel || entry.Level == zapcore.WarnLevel {
			
			if notifier == nil {
				L().Error("bot notifier not set, cannot send embed")
				return
			}

			ch := notifier.GetChannelFromName("error")
			if ch == "" {
				return
			}
			embed := embeds.ErrorEmbed(&embeds.ErrorEntry{
				Level: entry.Level.String(),
				Msg:   entry.Message,
				TS:    entry.Time.Format("2006-01-02 15:04:05"),
			})
			if err := notifier.SendEmbedToChannel(ch, "", &embed); err != nil {
				L().Error("failed to send embed", zap.Error(err))
			}
		}
	}()

	return nil
}

func (s *serverCore) Sync() error {
	return s.originalCore.Sync()
}



func InitServerLogger() error {
	if _, err := os.Stat("logs"); os.IsNotExist(err) {
		if err := os.Mkdir("logs", AppendCode); err != nil {
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
		encoder:      fileEncoder,
		originalCore: core,
	}

	log := zap.New(zapcore.NewTee(
		serverLogger,
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), zapcore.DebugLevel),
	))

	Init(log)
	return nil
}
