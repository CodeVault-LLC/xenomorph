package agentquic

import (
	"fmt"

	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

type ingressDecoder func([]byte) (IngressMessage, error)

var ingressDecoders = map[wire.MessageType]ingressDecoder{
	wire.MessageHeartbeat:        decodeHeartbeat,
	wire.MessageAttestation:      decodeAttestation,
	wire.MessageLogEntry:         decodeLogEntry,
	wire.MessageCommandResult:    decodeCommandResult,
	wire.MessageCommandState:     decodeCommandState,
	wire.MessageTransferOpen:     decodeTransferOpen,
	wire.MessageTransferChunk:    decodeTransferChunk,
	wire.MessageTransferFinalize: decodeTransferFinalize,
	wire.MessageTransferAbort:    decodeTransferAbort,
	wire.MessageMediaOpen:        decodeMediaOpen,
	wire.MessageMediaFrame:       decodeMediaFrame,
	wire.MessageMediaClose:       decodeMediaClose,
}

func decodeIngressMessage(frame wire.Frame) (IngressMessage, error) {
	decode, exists := ingressDecoders[frame.Header.Type]
	if !exists {
		return IngressMessage{}, wire.ErrUnexpectedMessage
	}

	message, err := decode(frame.Body)
	if err != nil {
		return IngressMessage{}, fmt.Errorf("decode %d body: %w", frame.Header.Type, err)
	}

	message.Type = frame.Header.Type

	return message, nil
}

func decodeHeartbeat(body []byte) (IngressMessage, error) {
	message := &wire.Heartbeat{}
	return IngressMessage{Heartbeat: message}, message.UnmarshalBinary(body)
}

func decodeAttestation(body []byte) (IngressMessage, error) {
	message := &wire.Attestation{}
	return IngressMessage{Attestation: message}, message.UnmarshalBinary(body)
}

func decodeLogEntry(body []byte) (IngressMessage, error) {
	message := &wire.LogEntry{}
	if err := message.UnmarshalBinary(body); err != nil {
		return IngressMessage{}, err
	}

	return IngressMessage{LogEntry: message}, wire.ValidateLogEntry(*message)
}

func decodeCommandResult(body []byte) (IngressMessage, error) {
	message := &wire.CommandResult{}
	if err := message.UnmarshalBinary(body); err != nil {
		return IngressMessage{}, err
	}

	return IngressMessage{CommandResult: message}, wire.ValidateCommandResult(*message)
}

func decodeCommandState(body []byte) (IngressMessage, error) {
	message := &wire.CommandState{}
	return IngressMessage{CommandState: message}, message.UnmarshalBinary(body)
}

func decodeTransferOpen(body []byte) (IngressMessage, error) {
	message := &wire.TransferOpen{}
	return IngressMessage{TransferOpen: message}, message.UnmarshalBinary(body)
}

func decodeTransferChunk(body []byte) (IngressMessage, error) {
	message := &wire.TransferChunk{}
	if err := message.UnmarshalBinary(body); err != nil {
		return IngressMessage{}, err
	}

	return IngressMessage{TransferChunk: message}, wire.ValidateTransferChunk(*message)
}

func decodeTransferFinalize(body []byte) (IngressMessage, error) {
	message := &wire.TransferFinalize{}
	return IngressMessage{TransferFinal: message}, message.UnmarshalBinary(body)
}

func decodeTransferAbort(body []byte) (IngressMessage, error) {
	message := &wire.TransferAbort{}
	return IngressMessage{TransferAbort: message}, message.UnmarshalBinary(body)
}

func decodeMediaOpen(body []byte) (IngressMessage, error) {
	message := &wire.MediaOpen{}
	return IngressMessage{MediaOpen: message}, message.UnmarshalBinary(body)
}

func decodeMediaFrame(body []byte) (IngressMessage, error) {
	message := &wire.MediaFrame{}
	if err := message.UnmarshalBinary(body); err != nil {
		return IngressMessage{}, err
	}

	return IngressMessage{MediaFrame: message}, wire.ValidateMediaFrame(*message)
}

func decodeMediaClose(body []byte) (IngressMessage, error) {
	message := &wire.MediaClose{}
	return IngressMessage{MediaClose: message}, message.UnmarshalBinary(body)
}
