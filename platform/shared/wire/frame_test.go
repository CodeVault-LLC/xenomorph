package wire

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	t.Parallel()

	codec, err := NewFrameCodec(1024)
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}

	operationID := [16]byte{1, 2, 3, 4}
	want := Frame{
		Header: FrameHeader{
			Type:                0x201,
			SchemaRevision:      1,
			Flags:               FlagAckRequired | FlagIsResponse | FlagHasCorrelation | FlagHasOperationID,
			Sequence:            300,
			CorrelationSequence: 127,
			OperationID:         operationID,
		},
		Body: []byte{4, 5, 6},
	}

	var encoded bytes.Buffer

	if err := codec.WriteFrame(&encoded, want); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	got, err := codec.ReadFrame(&encoded)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("frame = %#v, want %#v", got, want)
	}
}

func TestFrameRejectsMalformedHeader(t *testing.T) {
	t.Parallel()

	codec, err := NewFrameCodec(64)
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}

	tests := []struct {
		name    string
		encoded []byte
		want    error
	}{
		{name: "overlong length", encoded: []byte{0x84, 0x00, 1, 1, 0, 1}, want: ErrEncoding},
		{name: "overlong type", encoded: []byte{5, 0x81, 0, 1, 0, 1}, want: ErrEncoding},
		{name: "reserved flag", encoded: []byte{4, 1, 1, 0x40, 1}, want: ErrEncoding},
		{name: "response without correlation", encoded: []byte{4, 1, 1, byte(FlagIsResponse), 1}, want: ErrEncoding},
		{name: "zero sequence", encoded: []byte{4, 1, 1, 0, 0}, want: ErrEncoding},
		{name: "truncated payload", encoded: []byte{5, 1, 1, 0, 1}, want: ErrEncoding},
		{name: "above lane limit", encoded: []byte{65}, want: ErrLimit},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := codec.ReadFrame(bytes.NewReader(test.encoded)); err == nil || !errors.Is(err, test.want) {
				t.Fatalf("read error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestFrameLimitCheckedBeforeBodyAllocation(t *testing.T) {
	t.Parallel()

	codec, err := NewFrameCodec(1024)
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}

	input := &countingByteReader{data: []byte{0x81, 0x08}}
	if _, err := codec.ReadFrame(input); err == nil || !errors.Is(err, ErrLimit) {
		t.Fatalf("read error = %v, want ErrLimit", err)
	}

	if input.reads != 2 {
		t.Fatalf("reader consumed %d bytes, want only two-byte length", input.reads)
	}
}

type countingByteReader struct {
	data  []byte
	reads int
}

func (reader *countingByteReader) Read(destination []byte) (int, error) {
	for index := range destination {
		value, err := reader.ReadByte()
		if err != nil {
			return index, err
		}

		destination[index] = value
	}

	return len(destination), nil
}

func (reader *countingByteReader) ReadByte() (byte, error) {
	if reader.reads >= len(reader.data) {
		return 0, errors.New("end")
	}

	value := reader.data[reader.reads]
	reader.reads++

	return value, nil
}
