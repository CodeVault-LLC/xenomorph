package types

const (
	// Message types used in the protocol
	MsgConnect byte = 0x01 // Initial connection message
	// Handshake message, used to establish a secure connection
	MsgHandshake byte = 0x02

	// Used at the start of a connection to register the client with the server
	MsgRegistration byte = 0x03 // Registration message

	// Command messages, used for sending commands to the server
	MsgCommand byte = 0x04

	// Command response messages, used to send responses back to the client
	MsgCommandResponse byte = 0x05

	// File transfer messages, used for starting the file upload
	MsgFileStart byte = 0x06
	// File chunk messages, used to send parts of a file
	MsgFileChunk byte = 0x07
	// File end message, used to indicate the end of a file transfer
	MsgFileEnd byte = 0x08

	// Acknowledgment message, used to confirm receipt of a message
	MsgAck byte = 0x09

	// Ping message, used to check if the connection is still alive
	MsgPing byte = 0x010 // Ping message to keep the connection alive
	// Pong message, used to respond to a ping
	MsgPong byte = 0x011 // Pong message to respond to a ping
)
