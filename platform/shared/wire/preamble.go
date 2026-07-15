package wire

import (
	"fmt"
	"io"
)

const preambleSize = 6

var preambleMagic = [2]byte{'X', 'B'}

// StreamKind identifies the application role of a QUIC stream.
type StreamKind uint8

const (
	// StreamControl is the client-initiated bidirectional session-control lane.
	StreamControl StreamKind = iota
	// StreamEvents is the client-initiated unidirectional event lane.
	StreamEvents
	// StreamCommands is the server-initiated unidirectional command lane.
	StreamCommands
	// StreamTransfer is an operation-scoped bidirectional transfer lane.
	StreamTransfer
	// StreamTerminal is the reserved gateway-initiated terminal lane.
	StreamTerminal
	// StreamMedia is the client-initiated reliable screen-media lane.
	StreamMedia
	// StreamDiagnostics is a non-production conformance lane.
	StreamDiagnostics
)

// Preamble is the fixed six-byte prefix of every XBP application stream.
type Preamble struct {
	Kind StreamKind
}

// WritePreamble writes the canonical stream prefix for kind.
func WritePreamble(writer io.Writer, kind StreamKind) error {
	if writer == nil {
		return fmt.Errorf("write XBP preamble: %w: nil writer", ErrEncoding)
	}
	if !kind.valid() {
		return fmt.Errorf("write XBP preamble: %w: stream kind %d", ErrUnexpectedMessage, kind)
	}
	encoded := [preambleSize]byte{
		preambleMagic[0], preambleMagic[1], ProtocolMajor, byte(kind), StreamRevision, 0,
	}
	if _, err := writer.Write(encoded[:]); err != nil {
		return fmt.Errorf("write XBP preamble: %w", err)
	}
	return nil
}

// ReadPreamble reads and validates one canonical stream prefix.
func ReadPreamble(reader io.Reader) (Preamble, error) {
	if reader == nil {
		return Preamble{}, fmt.Errorf("read XBP preamble: %w: nil reader", ErrEncoding)
	}
	var encoded [preambleSize]byte
	if _, err := io.ReadFull(reader, encoded[:]); err != nil {
		return Preamble{}, fmt.Errorf("read XBP preamble: %w", err)
	}
	if encoded[0] != preambleMagic[0] || encoded[1] != preambleMagic[1] {
		return Preamble{}, fmt.Errorf("read XBP preamble: %w: wrong magic", ErrEncoding)
	}
	if encoded[2] != ProtocolMajor || encoded[4] != StreamRevision || encoded[5] != 0 {
		return Preamble{}, fmt.Errorf("read XBP preamble: %w: unsupported version or flags", ErrEncoding)
	}
	kind := StreamKind(encoded[3])
	if !kind.valid() {
		return Preamble{}, fmt.Errorf("read XBP preamble: %w: stream kind %d", ErrUnexpectedMessage, kind)
	}
	return Preamble{Kind: kind}, nil
}

func (kind StreamKind) valid() bool {
	return kind <= StreamDiagnostics
}
