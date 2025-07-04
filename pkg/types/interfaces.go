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
	GetCommand(id string) (CommandData, error)
	StoreCommand(id string, cmd CommandData)
	DeleteCommand(id string)
}
	
type ClientController interface {
	Read() (byte, byte, uint32, []byte, error)
	Send(msgType byte, flags byte, msgID uint32, payload []byte)
}