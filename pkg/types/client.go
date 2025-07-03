package types

import (
	"net"

	"github.com/codevault-llc/xenomorph/internal/secure"
)

type ConnectData struct {
	// The computers UUID (Universally Unique Identifier)
	UUID string `json:"uuid"`
}

// Used in the client list, and stores only the essential data of a client.
type ClientDataLite struct {
	// The computers UUID (Universally Unique Identifier)
	UUID string `json:"uuid"`
	// Address of the client
	Address net.Addr `json:"-"`
	// Socket
	Socket net.Conn `json:"-"`
}

// ClientSession represents a client session with essential information.
// It includes the client's UUID, address, connection, and secure connection manager.
type ClientSession struct {
	// The computers UUID (Universally Unique Identifier)
	UUID string `json:"uuid"`
	// Address of the client
	Address net.Addr `json:"address"`
	// Conn
	Conn net.Conn `json:"-"`
	// Secure connection manager
	Sec *secure.Sec `json:"-"`
}