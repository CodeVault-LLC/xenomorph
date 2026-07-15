package wire

import (
	"bytes"
	"errors"
	"testing"
)

func TestPreambleRoundTrip(t *testing.T) {
	t.Parallel()

	for kind := StreamControl; kind <= StreamDiagnostics; kind++ {
		kind := kind
		t.Run(string(rune('0'+kind)), func(t *testing.T) {
			t.Parallel()
			var encoded bytes.Buffer
			if err := WritePreamble(&encoded, kind); err != nil {
				t.Fatalf("write preamble: %v", err)
			}
			got, err := ReadPreamble(&encoded)
			if err != nil {
				t.Fatalf("read preamble: %v", err)
			}
			if got.Kind != kind {
				t.Fatalf("kind = %d, want %d", got.Kind, kind)
			}
		})
	}
}

func TestReadPreambleRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	valid := []byte{'X', 'B', ProtocolMajor, byte(StreamControl), StreamRevision, 0}
	tests := []struct {
		name  string
		index int
		value byte
	}{
		{name: "magic", index: 0, value: 'Y'},
		{name: "major", index: 2, value: 2},
		{name: "kind", index: 3, value: 0x3f},
		{name: "revision", index: 4, value: 2},
		{name: "flags", index: 5, value: 1},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			encoded := append([]byte(nil), valid...)
			encoded[test.index] = test.value
			if _, err := ReadPreamble(bytes.NewReader(encoded)); err == nil ||
				(!errors.Is(err, ErrEncoding) && !errors.Is(err, ErrUnexpectedMessage)) {
				t.Fatalf("read error = %v", err)
			}
		})
	}
}
