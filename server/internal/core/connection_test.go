package core

import (
	"testing"

	"github.com/codevault-llc/xenomorph/tests"
)

func TestServer_handleConnection(t *testing.T) {
	conn, err := tests.ConnectToSocket("localhost:8080")
	if err != nil {
		t.Errorf("Failed to connect to socket: %v", err)
	}

	defer tests.CloseConnection(conn)
}
