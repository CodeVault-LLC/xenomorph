package main

import (
	"fmt"
	"path/filepath"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/config"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/fileworkspace"
)

func buildFileWorkspace(cfg config.GatewayConfig, queue *command.Queue) (*fileworkspace.Service, error) {
	store, err := fileworkspace.NewStore(filepath.Join(cfg.StatePath, "file-operations.json"))
	if err != nil {
		return nil, fmt.Errorf("file workspace store setup: %w", err)
	}

	service, err := fileworkspace.NewService(queue, store)
	if err != nil {
		return nil, fmt.Errorf("file workspace service setup: %w", err)
	}

	return service, nil
}
