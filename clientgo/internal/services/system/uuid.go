package system

import (
	"github.com/codevault-llc/xenomorph-client/pkg/logger"
	"github.com/denisbrodbeck/machineid"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// GetUUID retrieves the unique identifier for the machine.
func GetUUID() string {
	id, err := machineid.ID()
  if err != nil {
		id = uuid.New().String()
		logger.GetLogger().Error("Failed to get machine ID, using fallback UUID", zap.Error(err), zap.String("uuid", id))
	}

	return id
}