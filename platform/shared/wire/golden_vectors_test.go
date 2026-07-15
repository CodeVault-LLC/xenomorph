package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

type goldenVectorCase struct {
	name        string
	messageType MessageType
	stream      StreamKind
	newMessage  func() generatedBinaryMessage
}

var structuralGoldenVectorCases = []goldenVectorCase{
	{name: "client_hello", messageType: MessageClientHello, stream: StreamControl, newMessage: func() generatedBinaryMessage { return &ClientHello{} }},
	{name: "server_hello", messageType: MessageServerHello, stream: StreamControl, newMessage: func() generatedBinaryMessage { return &ServerHello{} }},
	{name: "ping", messageType: MessagePing, stream: StreamControl, newMessage: func() generatedBinaryMessage { return &Ping{} }},
	{name: "pong", messageType: MessagePong, stream: StreamControl, newMessage: func() generatedBinaryMessage { return &Pong{} }},
	{name: "message_ack", messageType: MessageMessageAck, stream: StreamControl, newMessage: func() generatedBinaryMessage { return &MessageAck{} }},
	{name: "protocol_error", messageType: MessageProtocolError, stream: StreamControl, newMessage: func() generatedBinaryMessage { return &ProtocolError{} }},
	{name: "drain", messageType: MessageDrain, stream: StreamControl, newMessage: func() generatedBinaryMessage { return &Drain{} }},
	{name: "session_replaced", messageType: MessageSessionReplaced, stream: StreamControl, newMessage: func() generatedBinaryMessage { return &SessionReplaced{} }},
	{name: "heartbeat", messageType: MessageHeartbeat, stream: StreamEvents, newMessage: func() generatedBinaryMessage { return &Heartbeat{} }},
	{name: "attestation", messageType: MessageAttestation, stream: StreamEvents, newMessage: func() generatedBinaryMessage { return &Attestation{} }},
	{name: "log_entry", messageType: MessageLogEntry, stream: StreamEvents, newMessage: func() generatedBinaryMessage { return &LogEntry{} }},
	{name: "command", messageType: MessageCommand, stream: StreamCommands, newMessage: func() generatedBinaryMessage { return &Command{} }},
	{name: "command_result", messageType: MessageCommandResult, stream: StreamEvents, newMessage: func() generatedBinaryMessage { return &CommandResult{} }},
	{name: "command_cancel", messageType: MessageCommandCancel, stream: StreamCommands, newMessage: func() generatedBinaryMessage { return &CommandCancel{} }},
	{name: "command_state", messageType: MessageCommandState, stream: StreamEvents, newMessage: func() generatedBinaryMessage { return &CommandState{} }},
	{name: "transfer_open", messageType: MessageTransferOpen, stream: StreamTransfer, newMessage: func() generatedBinaryMessage { return &TransferOpen{} }},
	{name: "transfer_chunk", messageType: MessageTransferChunk, stream: StreamTransfer, newMessage: func() generatedBinaryMessage { return &TransferChunk{} }},
	{name: "transfer_chunk_ack", messageType: MessageTransferChunkAck, stream: StreamTransfer, newMessage: func() generatedBinaryMessage { return &TransferChunkAck{} }},
	{name: "transfer_finalize", messageType: MessageTransferFinalize, stream: StreamTransfer, newMessage: func() generatedBinaryMessage { return &TransferFinalize{} }},
	{name: "transfer_abort", messageType: MessageTransferAbort, stream: StreamTransfer, newMessage: func() generatedBinaryMessage { return &TransferAbort{} }},
	{name: "media_open", messageType: MessageMediaOpen, stream: StreamMedia, newMessage: func() generatedBinaryMessage { return &MediaOpen{} }},
	{name: "media_frame", messageType: MessageMediaFrame, stream: StreamMedia, newMessage: func() generatedBinaryMessage { return &MediaFrame{} }},
	{name: "media_close", messageType: MessageMediaClose, stream: StreamMedia, newMessage: func() generatedBinaryMessage { return &MediaClose{} }},
}

type goldenVectorMetadata struct {
	Message        string `json:"message"`
	Stream         string `json:"stream"`
	MessageType    uint16 `json:"message_type"`
	SchemaRevision uint8  `json:"schema_revision"`
	Flags          uint8  `json:"flags"`
	Sequence       uint64 `json:"sequence"`
	Fields         []any  `json:"fields"`
}

func TestGeneratedStructuralGoldenVectors(t *testing.T) {
	t.Parallel()

	for _, test := range structuralGoldenVectorCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertStructuralGoldenVector(t, test)
		})
	}
}

func assertStructuralGoldenVector(t *testing.T, test goldenVectorCase) {
	t.Helper()

	prefix := "testdata/golden/v1/" + test.name + "_structural_minimum"

	encoded, err := os.ReadFile(prefix + ".bin") // #nosec G304 -- the path is constructed from a static registered fixture name.
	if err != nil {
		t.Fatalf("read binary golden vector: %v", err)
	}

	metadata := readGoldenVectorMetadata(t, prefix+".json")
	descriptor := assertGoldenVectorMetadata(t, test, metadata)
	frame, codec := decodeGoldenVectorFrame(t, test.stream, encoded)
	assertGoldenVectorHeader(t, test, metadata, descriptor, frame)
	assertCanonicalGoldenVector(t, test, encoded, frame, codec)
}

func readGoldenVectorMetadata(t *testing.T, path string) goldenVectorMetadata {
	t.Helper()

	metadataData, err := os.ReadFile(path) // #nosec G304 -- the path is constructed from a static registered fixture name.
	if err != nil {
		t.Fatalf("read golden metadata: %v", err)
	}

	var metadata goldenVectorMetadata

	decoder := json.NewDecoder(bytes.NewReader(metadataData))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&metadata); err != nil {
		t.Fatalf("decode golden metadata: %v", err)
	}

	return metadata
}

func assertGoldenVectorMetadata(t *testing.T, test goldenVectorCase, metadata goldenVectorMetadata) MessageDescriptor {
	t.Helper()

	descriptor, ok := DescriptorForMessage(test.messageType)
	if !ok || descriptor.Stream != test.stream || metadata.Message != descriptor.Name ||
		metadata.Stream != streamName(test.stream) || metadata.MessageType != uint16(test.messageType) ||
		metadata.SchemaRevision != descriptor.Revision || metadata.Sequence != 1 {
		t.Fatalf("golden metadata does not match registry: %+v", metadata)
	}

	return descriptor
}

func decodeGoldenVectorFrame(t *testing.T, stream StreamKind, encoded []byte) (Frame, FrameCodec) {
	t.Helper()

	specification, ok := SpecificationForStream(stream)
	if !ok {
		t.Fatal("golden stream is not registered")
	}

	codec, err := NewFrameCodec(specification.MaximumFrameBytes)
	if err != nil {
		t.Fatalf("create frame codec: %v", err)
	}

	frame, err := codec.ReadFrame(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode independently generated frame: %v", err)
	}

	return frame, codec
}

func assertGoldenVectorHeader(
	t *testing.T,
	test goldenVectorCase,
	metadata goldenVectorMetadata,
	descriptor MessageDescriptor,
	frame Frame,
) {
	t.Helper()

	if frame.Header.Type != test.messageType || frame.Header.SchemaRevision != descriptor.Revision ||
		uint8(frame.Header.Flags) != metadata.Flags || frame.Header.Sequence != metadata.Sequence {
		t.Fatalf("golden frame header does not match metadata: %+v", frame.Header)
	}
}

func assertCanonicalGoldenVector(t *testing.T, test goldenVectorCase, encoded []byte, frame Frame, codec FrameCodec) {
	t.Helper()

	message := test.newMessage()
	if err := message.UnmarshalBinary(frame.Body); err != nil {
		t.Fatalf("decode independently generated body: %v", err)
	}

	body, err := message.MarshalBinary()
	if err != nil {
		t.Fatalf("re-encode golden body: %v", err)
	}

	var canonical bytes.Buffer

	frame.Body = body
	if err := codec.WriteFrame(&canonical, frame); err != nil {
		t.Fatalf("re-encode golden frame: %v", err)
	}

	if !bytes.Equal(canonical.Bytes(), encoded) {
		t.Fatalf("canonical frame differs: got % x want % x", canonical.Bytes(), encoded)
	}
}

func streamName(kind StreamKind) string {
	specification, ok := SpecificationForStream(kind)
	if !ok {
		return fmt.Sprintf("unregistered-%d", kind)
	}

	return specification.Name
}
