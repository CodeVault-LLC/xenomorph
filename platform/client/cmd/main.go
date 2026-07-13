// Command entry point for the remote support agent.
package main

import (
	"os"
	"time"
)

const (
	heartbeatInterval time.Duration = 500 * time.Millisecond
	routineCount      int           = 2
)

func run() int {
	ac, err := setupApp()
	if err != nil {
		return 1
	}

	isNewAgent, err := stage1Auth(ac)
	if err != nil {
		shutdown(ac)
		return 1
	}

	if err := stage2Entry(ac, isNewAgent); err != nil {
		shutdown(ac)
		return 1
	}

	if err := runRuntimeLoops(ac); err != nil {
		reportClientLog(ac, "ERROR", "client.runtime", err.Error())
		shutdown(ac)
		return 1
	}

	return 0
}

func runRuntimeLoops(ac *appContext) error {
	errCh := make(chan error, routineCount)
	go func() {
		errCh <- runHeartbeatLoop(ac)
	}()
	go func() {
		errCh <- runCommandLoop(ac)
	}()

	return <-errCh
}

func runHeartbeatLoop(ac *appContext) error {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := ac.ag.SendHeartbeat(); err != nil {
			reportClientLog(ac, "ERROR", "client.heartbeat", err.Error())
			return err
		}
	}

	return nil
}

func runCommandLoop(ac *appContext) error {
	for {
		cmd, err := ac.ag.PollNextCommand()
		if err != nil {
			reportClientLog(ac, "ERROR", "client.command.poll", err.Error())
			return err
		}

		if cmd == nil {
			continue
		}

		if err := processCommand(ac, cmd); err != nil {
			reportClientLog(ac, "ERROR", "client.command.process", err.Error())
			return err
		}
	}
}

func main() {
	os.Exit(run())
}
