// Command entry point for the remote support agent.
package main

import (
	"os"
	"time"
)

const routineCount = 2

func run() int {
	ac, err := setupApp()
	if err != nil {
		return 1
	}
	reportClientLog(ac, "INFO", "client.runtime", "event=runtime_started")

	requiresAttestation, err := authenticateDevice(ac)
	if err != nil {
		reportClientLog(ac, "ERROR", "client.authentication", "event=authentication_failed")
		shutdown(ac)
		return 1
	}
	reportClientLog(ac, "INFO", "client.authentication", "event=authentication_succeeded")

	if err := attestEndpoint(ac, requiresAttestation); err != nil {
		reportClientLog(ac, "ERROR", "client.attestation", "event=attestation_failed")
		shutdown(ac)
		return 1
	}
	if requiresAttestation {
		reportClientLog(ac, "INFO", "client.attestation", "event=attestation_submitted")
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
	ticker := time.NewTicker(ac.heartbeatInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := ac.transport.SendHeartbeat(); err != nil {
			reportClientLog(ac, "ERROR", "client.heartbeat", "event=heartbeat_failed")
			return err
		}
	}

	return nil
}

func runCommandLoop(ac *appContext) error {
	for {
		cmd, err := ac.transport.PollNextCommand()
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
