package messages

import (
	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (m *MessageCore) handlePing(uuid string, _ *common.Message) {
	logger.Log.Debug("Received keep-alive message", zap.String("uuid", uuid))
}
