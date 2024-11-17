package main

import (
	"fmt"
	"os"

	"github.com/codevault-llc/xenomorph/config"
	"github.com/codevault-llc/xenomorph/internal/bot"
	"github.com/codevault-llc/xenomorph/internal/core"
	"github.com/codevault-llc/xenomorph/internal/core/messages"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	log, err := logger.InitLogger()
	if err != nil {
		fmt.Println("Failed to initialize logger")
		os.Exit(1)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Error("Error loading .env file", zap.Error(err))
	}

	server := core.NewServer(cfg.ServerPort, nil, nil)
	botInstance, err := bot.NewBot(cfg.DiscordToken, server)
	if err != nil {
		logger.Log.Error("Failed to create bot", zap.Error(err))
	}

	messageInstance := messages.NewMessageCore(server, botInstance)

	server.MessageController = messageInstance
	server.BotController = botInstance
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
