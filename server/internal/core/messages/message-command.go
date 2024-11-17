package messages

import (
	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (m *MessageCore) handleCommand(_ string, msg *common.Message) {
	logger.Log.Info("Processing command message", zap.Any("data", msg.Data))
}
