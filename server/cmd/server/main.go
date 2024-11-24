package main

import (
	"fmt"

	"github.com/codevault-llc/xenomorph/config"
	"github.com/codevault-llc/xenomorph/internal/bot"
	"github.com/codevault-llc/xenomorph/internal/core"
	"github.com/codevault-llc/xenomorph/internal/core/messages"
	"github.com/codevault-llc/xenomorph/internal/handler"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	server := core.NewServer(cfg.ServerPort)
	botInstance, err := bot.NewBot(cfg.DiscordToken, server)
	if err != nil {
		panic(err)
	}

	_, err = logger.InitLogger(botInstance)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	messageInstance := messages.NewMessageCore(server, botInstance)
	handlerInstance := handler.NewHandler(server, messageInstance)

	server.MessageController = messageInstance
	server.BotController = botInstance
	server.Handler = handlerInstance

	logger.Log.Info("Bot and server initialized", zap.String("port", cfg.ServerPort))

	go func() {
		if err := botInstance.Run(); err != nil {
			logger.Log.Error("Bot failed to run", zap.Error(err))
		}
	}()

	if err := server.Start(); err != nil {
		logger.Log.Error("Server failed to start", zap.Error(err))
	}
}
