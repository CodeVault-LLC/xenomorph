package main

import (
	"fmt"

	"github.com/codevault-llc/xenomorph-client/internal/client"
	"github.com/codevault-llc/xenomorph-client/pkg/logger"
	"github.com/codevault-llc/xenomorph-client/pkg/utils"
	"go.uber.org/zap"
)

func main() {
	err := logger.NewLogger("development")
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	utils.RerunAsAdmin()

	client := client.NewClient("127.0.0.1:8080")
	if err := client.Run(); err != nil {
		logger.GetLogger().Error("Failed to connect to server", zap.Error(err))
		return
	}
	defer client.Close()
}
