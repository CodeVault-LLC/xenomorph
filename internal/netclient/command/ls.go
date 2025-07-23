package command

import (
	"os/exec"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"go.uber.org/zap"
)

// This command is used to list files or directories in the current working directory.
func initLsCommand() {
	err := commandHandler.AddCommand("ls", func(client types.ClientController, msgID uint32, command types.Command) {
		logger.L().Info("Received 'ls' command", zap.String("command", command.Name), zap.Strings("args", command.Args))

		// Run the command from the cli
		execCmd := exec.Command("ls", command.Args...)
		output, err := execCmd.CombinedOutput()
		if err != nil {
			logger.L().Error("Failed to execute 'ls' command", zap.Error(err), zap.String("command", command.Name), zap.Strings("args", command.Args))
			response := types.CommandResponse{
				ID: command.ID,
				Error:    err.Error(),
				Duration: 0, // Duration can be calculated if needed
			}
			// Send the error response back to the client
			client.Send(types.MsgCommandResponse, 0, msgID, []byte(response.ToJSON()))
			return
		}
		logger.L().Info("'ls' command executed successfully", zap.String("output", string(output)))
		response := types.CommandResponse{
			ID: command.ID,
			Output:   string(output),
			Error:    "",
			Duration: 0, // Duration can be calculated if needed
		}
		
		client.Send(types.MsgCommandResponse, 0, msgID, []byte(response.ToJSON()))
		logger.L().Info("Sent 'ls' command response", zap.Uint32("commandID", msgID), zap.String("output", response.Output))
	})
	
	if err != nil {
		logger.L().Error("Failed to register 'ls' command handler", zap.Error(err))
	}
}