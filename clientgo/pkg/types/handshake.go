package types

type HandshakePayload struct {
	Encryption string // "aes-gcm"
	Key        []byte // symmetric key material
}