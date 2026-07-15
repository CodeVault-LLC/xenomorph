package wire

const (
	// ALPN is the application protocol identifier for XBP major version 1.
	ALPN = "xenomorph-agent/1"
	// ProtocolMajor is the XBP major version selected by ALPN.
	ProtocolMajor uint8 = 1
	// ProtocolMinor is the only negotiated XBP minor version currently defined.
	ProtocolMinor uint16 = 0
	// StreamRevision is the canonical stream-preamble revision for XBP/1.
	StreamRevision uint8 = 1
)

// FrameFlag controls optional XBP frame-header fields and handling policy.
type FrameFlag uint8

const (
	// FlagAckRequired requests an application acknowledgement at the commit point.
	FlagAckRequired FrameFlag = 1 << iota
	// FlagIsResponse marks a response and requires FlagHasCorrelation.
	FlagIsResponse
	// FlagHasCorrelation includes CorrelationSequence in the frame header.
	FlagHasCorrelation
	// FlagHasOperationID includes a stable operation identifier in the header.
	FlagHasOperationID
	// FlagSensitive prevents diagnostic payload capture.
	FlagSensitive
	// FlagEndOperation marks the final frame for an operation.
	FlagEndOperation
)

const knownFrameFlags = FlagAckRequired | FlagIsResponse | FlagHasCorrelation |
	FlagHasOperationID | FlagSensitive | FlagEndOperation

// SenderRole identifies one side of an authenticated XBP session.
type SenderRole uint8

const (
	// SenderAgent identifies frames sent by the authenticated agent connection.
	SenderAgent SenderRole = iota + 1
	// SenderGateway identifies frames sent by the gateway session.
	SenderGateway
)
