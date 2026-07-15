package wire

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestCanonicalUvarintBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoded []byte
		want    uint64
		wantErr bool
	}{
		{name: "zero", encoded: []byte{0x00}, want: 0},
		{name: "one byte maximum", encoded: []byte{0x7f}, want: 127},
		{name: "two byte minimum", encoded: []byte{0x80, 0x01}, want: 128},
		{name: "uint64 maximum", encoded: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, want: math.MaxUint64},
		{name: "overlong zero", encoded: []byte{0x80, 0x00}, wantErr: true},
		{name: "overlong value", encoded: []byte{0x81, 0x00}, wantErr: true},
		{name: "truncated", encoded: []byte{0x80}, wantErr: true},
		{name: "overflow", encoded: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x02}, wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, width, err := decodeCanonicalUvarint(test.encoded, maxUint64VarintBytes)
			if test.wantErr {
				if err == nil || !errors.Is(err, ErrEncoding) {
					t.Fatalf("decode error = %v, want ErrEncoding", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}
			if got != test.want || width != len(test.encoded) {
				t.Fatalf("decode = (%d, %d), want (%d, %d)", got, width, test.want, len(test.encoded))
			}
		})
	}
}

func TestBuilderParserCanonicalBody(t *testing.T) { //nolint:cyclop // One round-trip test deliberately checks every primitive in wire order.
	t.Parallel()

	builder := NewBuilder(64)
	builder.Uint(300)
	builder.SInt(-42)
	builder.Bool(true)
	builder.Fixed32(42)
	builder.Fixed64(84)
	builder.String("agent", 16)
	builder.BytesField([]byte{1, 2, 3}, 3)
	builder.Presence(0x101, 9)
	builder.StringList([]string{"linux", "windows"}, 2, 16, 64)
	body, err := builder.Bytes()
	if err != nil {
		t.Fatalf("encode body: %v", err)
	}

	parser := NewParser(body)
	if got := parser.Uint(300); got != 300 {
		t.Fatalf("uint = %d, want 300", got)
	}
	if got := parser.SInt(42); got != -42 {
		t.Fatalf("sint = %d, want -42", got)
	}
	if !parser.Bool() {
		t.Fatal("bool = false, want true")
	}
	if got := parser.Fixed32(); got != 42 {
		t.Fatalf("fixed32 = %d, want 42", got)
	}
	if got := parser.Fixed64(); got != 84 {
		t.Fatalf("fixed64 = %d, want 84", got)
	}
	if got := parser.String(16); got != "agent" {
		t.Fatalf("string = %q, want agent", got)
	}
	if got := parser.BytesField(3); !reflect.DeepEqual(got, []byte{1, 2, 3}) {
		t.Fatalf("bytes = %v", got)
	}
	if got := parser.Presence(9); got != 0x101 {
		t.Fatalf("presence = %#x, want 0x101", got)
	}
	if got := parser.StringList(2, 16, 64); !reflect.DeepEqual(got, []string{"linux", "windows"}) {
		t.Fatalf("list = %v", got)
	}
	if err := parser.Done(); err != nil {
		t.Fatalf("body has trailing data: %v", err)
	}
}

func TestParserRejectsMalformedPrimitives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		body  []byte
		parse func(*Parser)
	}{
		{name: "invalid bool", body: []byte{2}, parse: func(parser *Parser) { parser.Bool() }},
		{name: "invalid UTF-8", body: []byte{1, 0xff}, parse: func(parser *Parser) { parser.String(1) }},
		{name: "NUL string", body: []byte{1, 0}, parse: func(parser *Parser) { parser.String(1) }},
		{name: "overlong presence", body: []byte{1, 0}, parse: func(parser *Parser) { parser.Presence(8) }},
		{name: "unknown presence bit", body: []byte{1, 0x80}, parse: func(parser *Parser) { parser.Presence(7) }},
		{name: "truncated bytes", body: []byte{2, 1}, parse: func(parser *Parser) { parser.BytesField(2) }},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parser := NewParser(test.body)
			test.parse(parser)
			if err := parser.Done(); err == nil || !errors.Is(err, ErrEncoding) {
				t.Fatalf("parse error = %v, want ErrEncoding", err)
			}
		})
	}
}

func TestBuilderRejectsBoundsBeforeReturningBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(*Builder)
	}{
		{name: "string", build: func(builder *Builder) { builder.String("too long", 3) }},
		{name: "unsigned integer", build: func(builder *Builder) { builder.BoundedUint(4, 3) }},
		{name: "signed integer", build: func(builder *Builder) { builder.BoundedSInt(-4, 3) }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			builder := NewBuilder(0)
			test.build(builder)
			if _, err := builder.Bytes(); err == nil || !errors.Is(err, ErrLimit) {
				t.Fatalf("encode error = %v, want ErrLimit", err)
			}
		})
	}
}
