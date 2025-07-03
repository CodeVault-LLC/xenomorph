package common

const (
	MsgConnect      byte = 0x01 // Initial connection message
	MsgRegistration byte = 0x02 // Registration message

	MsgCommand   byte = 0x03
	MsgFileStart byte = 0x04
	MsgFileChunk byte = 0x05
	MsgFileEnd   byte = 0x06
	MsgAck       byte = 0x07
)
