package wire

import "errors"

var (
	// ErrEncoding identifies a noncanonical or structurally invalid XBP value.
	ErrEncoding = errors.New("invalid XBP encoding")
	// ErrLimit identifies an XBP value that exceeds its declared resource bound.
	ErrLimit = errors.New("XBP limit exceeded")
	// ErrUnexpectedMessage identifies a message that is illegal for its stream.
	ErrUnexpectedMessage = errors.New("unexpected XBP message")
	// ErrReplay identifies a duplicate, stale, or pathological session sequence.
	ErrReplay = errors.New("XBP replay violation")
)

// ApplicationErrorCode is an XBP connection or stream close code.
type ApplicationErrorCode uint64

const (
	// ApplicationNoError denotes a planned shutdown or completed drain.
	ApplicationNoError ApplicationErrorCode = 0x100
	// ApplicationVersion denotes incompatible protocol negotiation.
	ApplicationVersion ApplicationErrorCode = 0x101
	// ApplicationStreamPreamble denotes an invalid stream preamble or topology.
	ApplicationStreamPreamble ApplicationErrorCode = 0x102
	// ApplicationFrameEncoding denotes a noncanonical or malformed frame.
	ApplicationFrameEncoding ApplicationErrorCode = 0x103
	// ApplicationLimit denotes an exceeded protocol resource bound.
	ApplicationLimit ApplicationErrorCode = 0x104
	// ApplicationUnexpectedMessage denotes a message illegal for stream or state.
	ApplicationUnexpectedMessage ApplicationErrorCode = 0x105
	// ApplicationReplay denotes a duplicate, stale, or pathological sequence.
	ApplicationReplay ApplicationErrorCode = 0x106
	// ApplicationSessionReplaced denotes a connection fenced by a newer session.
	ApplicationSessionReplaced ApplicationErrorCode = 0x107
	// ApplicationAuthState denotes application work attempted before readiness.
	ApplicationAuthState ApplicationErrorCode = 0x108
	// ApplicationInternal denotes a bounded internal failure without peer detail.
	ApplicationInternal ApplicationErrorCode = 0x109
)
