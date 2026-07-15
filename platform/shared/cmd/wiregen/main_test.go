package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidateSchemaRejectsRegistryAndCompatibilityViolations(t *testing.T) {
	t.Parallel()

	base := protocolSchema{
		Protocol: protocolDefinition{Name: "XBP", Major: 1, Minor: 0, ALPN: "xenomorph-agent/1"},
		Streams: []streamDefinition{{Name: "events", GoName: "Events", Code: 1, MaximumFrameBytes: 1024,
			Initiator: "agent", Direction: "unidirectional"}},
		Messages: []messageDefinition{{Name: "Event", GoName: "Event", ID: 256, Stream: "events", Revision: 1,
			PresenceBits: 2, Fields: []fieldDefinition{
				{Name: "detail", GoName: "Detail", Type: "string", Maximum: 32, OptionalBit: uint8Pointer(0), Trust: "client-authored"},
				{Name: "count", GoName: "Count", Type: "uint", Maximum: 10, OptionalBit: uint8Pointer(1), Trust: "client-authored"},
			}}},
	}
	history := registryHistory{Major: 1, Assigned: map[string]string{"256": "Event"},
		Compatibility: map[string]string{"256": messageCompatibilitySignature(base.Messages[0])}, Tombstoned: map[string]string{}}

	if err := validateSchema(base, history); err != nil {
		t.Fatalf("valid schema rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*protocolSchema, *registryHistory)
	}{
		{name: "ID history mismatch", mutate: func(_ *protocolSchema, history *registryHistory) { history.Assigned["256"] = "Other" }},
		{name: "tombstone reuse", mutate: func(_ *protocolSchema, history *registryHistory) { history.Tombstoned["256"] = "Retired" }},
		{name: "duplicate optional bit", mutate: func(schema *protocolSchema, _ *registryHistory) {
			schema.Messages[0].Fields = append(schema.Messages[0].Fields, fieldDefinition{Name: "other", GoName: "Other",
				Type: "string", Maximum: 32, OptionalBit: uint8Pointer(0), Trust: "client-authored"})
		}},
		{name: "unbounded field", mutate: func(schema *protocolSchema, _ *registryHistory) { schema.Messages[0].Fields[0].Maximum = 0 }},
		{name: "bound widening", mutate: func(schema *protocolSchema, _ *registryHistory) { schema.Messages[0].Fields[0].Maximum = 64 }},
		{name: "field reorder", mutate: func(schema *protocolSchema, _ *registryHistory) {
			schema.Messages[0].Fields[0], schema.Messages[0].Fields[1] = schema.Messages[0].Fields[1], schema.Messages[0].Fields[0]
		}},
		{name: "required field insertion", mutate: func(schema *protocolSchema, _ *registryHistory) {
			schema.Messages[0].Fields = append(schema.Messages[0].Fields, fieldDefinition{Name: "required", GoName: "Required",
				Type: "uint", Maximum: 10, Trust: "client-authored"})
		}},
		{name: "revision changed", mutate: func(schema *protocolSchema, _ *registryHistory) { schema.Messages[0].Revision = 2 }},
		{name: "unknown stream", mutate: func(schema *protocolSchema, _ *registryHistory) { schema.Messages[0].Stream = "missing" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schemaCopy := cloneSchema(base)
			historyCopy := registryHistory{Major: history.Major, Assigned: cloneMap(history.Assigned),
				Compatibility: cloneMap(history.Compatibility), Tombstoned: cloneMap(history.Tombstoned)}
			test.mutate(&schemaCopy, &historyCopy)

			if err := validateSchema(schemaCopy, historyCopy); err == nil {
				t.Fatal("invalid schema was accepted")
			}
		})
	}
}

func TestGenerationIsDeterministic(t *testing.T) {
	t.Parallel()

	schema, err := readJSONFile[protocolSchema]("../../protocol/xbp-v1.yaml")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	first, err := generateMessages(schema)
	if err != nil {
		t.Fatalf("first generation: %v", err)
	}

	second, err := generateMessages(schema)
	if err != nil {
		t.Fatalf("second generation: %v", err)
	}

	if !bytes.Equal(first, second) || !strings.HasPrefix(string(first), generatedHeader) {
		t.Fatal("message generation is not deterministic generated source")
	}
}

func uint8Pointer(value uint8) *uint8 { return &value }

func cloneSchema(source protocolSchema) protocolSchema {
	clone := source
	clone.Streams = append([]streamDefinition(nil), source.Streams...)
	clone.Messages = append([]messageDefinition(nil), source.Messages...)

	for index := range clone.Messages {
		clone.Messages[index].Fields = append([]fieldDefinition(nil), source.Messages[index].Fields...)
	}

	return clone
}

func cloneMap(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}

	return clone
}
