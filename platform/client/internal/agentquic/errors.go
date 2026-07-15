package agentquic

import "errors"

var (
	// ErrSecurityFailure means reconnect supervision must fail closed.
	ErrSecurityFailure = errors.New("QUIC security negotiation failed")
	// ErrDeliveryUncertain means bytes may have crossed the application commit boundary.
	ErrDeliveryUncertain = errors.New("QUIC message delivery is uncertain")
	// ErrSessionDraining means the gateway requested bounded session shutdown.
	ErrSessionDraining = errors.New("QUIC session is draining")
	// ErrSessionReplaced means another connection authenticated with this certificate.
	ErrSessionReplaced = errors.New("QUIC session was replaced")
)
