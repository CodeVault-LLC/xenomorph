package agentquic

import (
	"context"
	"strings"
	"testing"
)

func TestNilListenerRunFailsWithoutPanic(t *testing.T) {
	var listener *Listener
	err := listener.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "listener is nil") {
		t.Fatalf("Run error = %v, want nil-listener error", err)
	}
}
