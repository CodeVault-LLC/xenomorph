package wire

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestWorkedLogEntryGoldenVector(t *testing.T) {
	t.Parallel()

	want, err := os.ReadFile("testdata/golden/v1/log_entry_minimal.bin")
	if err != nil {
		t.Fatalf("read golden vector: %v", err)
	}

	body, err := (LogEntry{Level: 1, Component: 1, EventCode: 1}).MarshalBinary()
	if err != nil {
		t.Fatalf("encode log entry: %v", err)
	}

	codec, err := NewFrameCodec(1024)
	if err != nil {
		t.Fatalf("create frame codec: %v", err)
	}

	var encoded bytes.Buffer
	if err := codec.WriteFrame(&encoded, Frame{
		Header: FrameHeader{Type: MessageLogEntry, SchemaRevision: 1, Flags: FlagAckRequired, Sequence: 1},
		Body:   body,
	}); err != nil {
		t.Fatalf("encode frame: %v", err)
	}

	if !bytes.Equal(encoded.Bytes(), want) {
		t.Fatalf("frame = % x, want % x", encoded.Bytes(), want)
	}

	frame, err := codec.ReadFrame(bytes.NewReader(want))
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}

	var entry LogEntry
	if err := entry.UnmarshalBinary(frame.Body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if err := ValidateLogEntry(entry); err != nil {
		t.Fatalf("validate body: %v", err)
	}
}

func TestHeartbeatPresenceRoundTrip(t *testing.T) {
	t.Parallel()

	want := Heartbeat{
		Presence:         (1 << 1) | (1 << 8) | (1 << 28),
		Hostname:         "agent-host",
		CPULoadPPM:       500_000,
		GPUDevices:       []string{"gpu0", "gpu1"},
		ApplicationTypes: []ApplicationUsage{{Category: 1, Count: 4}},
	}

	body, err := want.MarshalBinary()
	if err != nil {
		t.Fatalf("encode heartbeat: %v", err)
	}

	var got Heartbeat
	if err := got.UnmarshalBinary(body); err != nil {
		t.Fatalf("decode heartbeat: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("heartbeat = %#v, want %#v", got, want)
	}

	if reencoded, err := got.MarshalBinary(); err != nil || !bytes.Equal(reencoded, body) {
		t.Fatalf("canonical re-encode = % x, %v; want % x", reencoded, err, body)
	}
}

func TestMessageDecoderRejectsTrailingBytes(t *testing.T) {
	t.Parallel()

	body, err := (Ping{Nonce: 42}).MarshalBinary()
	if err != nil {
		t.Fatalf("encode ping: %v", err)
	}

	body = append(body, 0)

	var ping Ping

	if err := ping.UnmarshalBinary(body); err == nil || !errors.Is(err, ErrEncoding) {
		t.Fatalf("decode error = %v, want ErrEncoding", err)
	}
}

func TestTransferChunkDigestValidation(t *testing.T) {
	t.Parallel()

	chunk := TransferChunk{DigestAlgorithm: 1, ChunkLength: 1, Data: []byte{1}}
	if err := ValidateTransferChunk(chunk); err == nil || !errors.Is(err, ErrEncoding) {
		t.Fatalf("validation error = %v, want digest rejection", err)
	}
}

func FuzzFrameDecoder(f *testing.F) {
	seed, err := os.ReadFile("testdata/golden/v1/log_entry_minimal.bin")
	if err != nil {
		f.Fatalf("read golden seed: %v", err)
	}

	f.Add(seed)
	f.Add([]byte{0x80, 0x00})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0x10})

	codec, err := NewFrameCodec(1 << 20)
	if err != nil {
		f.Fatalf("create frame codec: %v", err)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		frame, err := codec.ReadFrame(bytes.NewReader(data))
		if err != nil {
			return
		}

		var output bytes.Buffer
		if err := codec.WriteFrame(&output, frame); err != nil {
			t.Fatalf("accepted frame failed canonical encode: %v", err)
		}
	})
}

func FuzzPreambleDecoder(f *testing.F) {
	f.Add([]byte{'X', 'B', 1, 0, 1, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		preamble, err := ReadPreamble(bytes.NewReader(data))
		if err != nil {
			return
		}

		var output bytes.Buffer
		if err := WritePreamble(&output, preamble.Kind); err != nil {
			t.Fatalf("accepted preamble failed canonical encode: %v", err)
		}
	})
}

type generatedBinaryMessage interface {
	MarshalBinary() ([]byte, error)
	UnmarshalBinary([]byte) error
}

func FuzzGeneratedMessageDecoders(f *testing.F) {
	f.Add([]byte{0})
	f.Add([]byte{0x80, 0})
	f.Add(bytes.Repeat([]byte{0xff}, 32))

	factories := []func() generatedBinaryMessage{
		func() generatedBinaryMessage { return &ClientHello{} },
		func() generatedBinaryMessage { return &ServerHello{} },
		func() generatedBinaryMessage { return &Ping{} },
		func() generatedBinaryMessage { return &Pong{} },
		func() generatedBinaryMessage { return &MessageAck{} },
		func() generatedBinaryMessage { return &ProtocolError{} },
		func() generatedBinaryMessage { return &Drain{} },
		func() generatedBinaryMessage { return &SessionReplaced{} },
		func() generatedBinaryMessage { return &Heartbeat{} },
		func() generatedBinaryMessage { return &Attestation{} },
		func() generatedBinaryMessage { return &LogEntry{} },
		func() generatedBinaryMessage { return &Command{} },
		func() generatedBinaryMessage { return &CommandResult{} },
		func() generatedBinaryMessage { return &CommandCancel{} },
		func() generatedBinaryMessage { return &CommandState{} },
		func() generatedBinaryMessage { return &TransferOpen{} },
		func() generatedBinaryMessage { return &TransferChunk{} },
		func() generatedBinaryMessage { return &TransferChunkAck{} },
		func() generatedBinaryMessage { return &TransferFinalize{} },
		func() generatedBinaryMessage { return &TransferAbort{} },
		func() generatedBinaryMessage { return &MediaOpen{} },
		func() generatedBinaryMessage { return &MediaFrame{} },
		func() generatedBinaryMessage { return &MediaClose{} },
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		for _, factory := range factories {
			message := factory()
			if err := message.UnmarshalBinary(data); err != nil {
				continue
			}

			canonical, err := message.MarshalBinary()
			if err != nil {
				t.Fatalf("accepted message failed canonical encode: %v", err)
			}

			if err := factory().UnmarshalBinary(canonical); err != nil {
				t.Fatalf("canonical message failed decode: %v", err)
			}
		}
	})
}

func FuzzPrimitiveDecoders(f *testing.F) {
	f.Add([]byte{0})
	f.Add([]byte{0x80, 0})
	f.Add(bytes.Repeat([]byte{0xff}, 32))
	f.Fuzz(func(_ *testing.T, data []byte) {
		parsers := []*Parser{NewParser(data), NewParser(data), NewParser(data), NewParser(data), NewParser(data), NewParser(data)}
		_ = parsers[0].Uint(^uint64(0))
		_ = parsers[1].SInt(1 << 62)
		_ = parsers[2].Bool()
		_ = parsers[3].BytesField(1 << 20)
		_ = parsers[4].Presence(64)
		_ = parsers[5].StringList(64, 4096, 1<<20)
	})
}
