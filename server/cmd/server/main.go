package main

import (
	"fmt"

	"github.com/codevault-llc/xenomorph/config"
	"github.com/codevault-llc/xenomorph/internal/bot"
	"github.com/codevault-llc/xenomorph/internal/core"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	err = logger.NewLogger()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	botInstance, err := bot.NewBot(cfg.DiscordToken)
	if err != nil {
		panic(err)
	}

	logger.AddBot(botInstance)

	server := core.NewServer(cfg.ServerPort, botInstance)
	botInstance.AddServerController(server)

	server.BotController = botInstance

	logger.GetLogger().Info("Starting server", zap.String("port", cfg.ServerPort))

	go func() {
		if err := botInstance.Run(); err != nil {
			logger.GetLogger().Error("Bot failed to start", zap.Error(err))
		}
	}()

	if err := server.Start(); err != nil {
		logger.GetLogger().Error("Server failed to start", zap.Error(err))
	}
}
