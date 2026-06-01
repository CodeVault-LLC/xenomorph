// Command entry point for the remote support agent.
package main

import (
	"os"
	"time"
)

const pollInterval = 5 * time.Second

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

	if err := runCommandLoop(ac); err != nil {
		shutdown(ac)
		return 1
	}

	return 0
}

func runCommandLoop(ac *appContext) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := ac.ag.SendHeartbeat(); err != nil {
			return err
		}

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
