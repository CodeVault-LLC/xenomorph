// Command entry point for the remote support agent.
package main

import (
	"os"
	"time"
)

const heartbeatInterval = 500 * time.Millisecond

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
		shutdown(ac)
		return 1
	}

	return 0
}

func runRuntimeLoops(ac *appContext) error {
	errCh := make(chan error, 2)
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
			return err
		}
	}

	return nil
}

func runCommandLoop(ac *appContext) error {
	for {
		cmd, err := ac.ag.PollNextCommand()
		if err != nil {
			return err
		}

		if cmd == nil {
			continue
		}

		if err := processCommand(ac, cmd); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	os.Exit(run())
}
