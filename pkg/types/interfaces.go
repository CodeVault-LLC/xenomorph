package types

// ServerController defines methods that the Bot can call on the Server.
type SessionController interface {
	Read() (byte, byte, uint32, []byte, error)
	Send(msgType byte, flags byte, msgID uint32, payload []byte) error
	Handle() error
	GetSessionId() string
}

// RegistryController defines methods that the Bot can call on the Registry.
type RegistryController interface {
	Get(id string) (*SessionController, error)
	GetCommand(id uint32) (CommandData, error)
	StoreCommand(id uint32, cmd CommandData)
	DeleteCommand(id uint32)
}
	
type ClientController interface {
	Read() (byte, byte, uint32, []byte, error)
	Send(msgType byte, flags byte, msgID uint32, payload []byte)
}