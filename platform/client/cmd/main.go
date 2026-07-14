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
	reportClientLog(ac, "INFO", "client.runtime", "event=runtime_started")

	isNewAgent, err := stage1Auth(ac)
	if err != nil {
		reportClientLog(ac, "ERROR", "client.authentication", "event=authentication_failed")
		shutdown(ac)
		return 1
	}
	reportClientLog(ac, "INFO", "client.authentication", "event=authentication_succeeded")

	if err := stage2Entry(ac, isNewAgent); err != nil {
		reportClientLog(ac, "ERROR", "client.onboarding", "event=entry_report_failed")
		shutdown(ac)
		return 1
	}
	if isNewAgent {
		reportClientLog(ac, "INFO", "client.onboarding", "event=entry_report_submitted")
	}

	if err := runRuntimeLoops(ac); err != nil {
		reportClientLog(ac, "ERROR", "client.runtime", "event=runtime_loop_failed")
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
			reportClientLog(ac, "ERROR", "client.heartbeat", "event=heartbeat_failed")
			return err
		}
	}

	return nil
}

func runCommandLoop(ac *appContext) error {
	for {
		cmd, err := ac.ag.PollNextCommand()
		if err != nil {
			reportClientLog(ac, "ERROR", "client.command", "event=command_poll_failed")
			return err
		}

		if cmd == nil {
			continue
		}
		reportClientLog(ac, "INFO", "client.command", "event=command_received")

		if err := processCommand(ac, cmd); err != nil {
			reportClientLog(ac, "ERROR", "client.command", "event=command_processing_failed")
			return err
		}
	}
}

func main() {
	os.Exit(run())
}
