package main

import (
	"fmt"

	"net/http"
	_ "net/http/pprof"

	"github.com/google/gops/agent"

	"github.com/codevault-llc/xenomorph/internal/config"
	"github.com/codevault-llc/xenomorph/internal/netserver"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	err = logger.InitServerLogger()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	if cfg.EnablePprof {
		go func() {
			logger.L().Info("Starting pprof server on port 6060")
			if err := http.ListenAndServe("localhost:6060", nil); err != nil {
				logger.L().Error("Failed to start pprof server", zap.Error(err))
			}
		}()
	}

	if cfg.EnableGops {
		if err := agent.Listen(agent.Options{}); err != nil {
			logger.L().Error("Failed to start gops agent", zap.Error(err))
			return
		}
		logger.L().Info("Gops agent started")
	}

	server := netserver.NewServer("127.0.0.1", cfg.ServerPort)

	logger.L().Info("Starting server", zap.String("port", cfg.ServerPort))

	if err := server.Start(); err != nil {
		logger.L().Error("Server failed to start", zap.Error(err))
	}
}
