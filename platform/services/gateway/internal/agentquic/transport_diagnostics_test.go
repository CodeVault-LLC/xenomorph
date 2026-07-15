package agentquic

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestBoundedDiagnosticWriterStopsAtRetentionLimit(t *testing.T) {
	t.Parallel()

	budget := &diagnosticCaptureBudget{}
	budget.remainingBytes.Store(4)
	metrics := &Metrics{}
	destination := &bytes.Buffer{}
	writer := &boundedDiagnosticWriter{writer: destination, budget: budget, metrics: metrics}

	written, err := writer.Write([]byte("abcdef"))
	if written != 4 || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Write() = (%d, %v), want (4, io.ErrShortWrite)", written, err)
	}
	if destination.String() != "abcd" {
		t.Fatalf("retained bytes = %q, want %q", destination.String(), "abcd")
	}
	if retained := metrics.Snapshot().QlogBytes; retained != 4 {
		t.Fatalf("qlog bytes = %d, want 4", retained)
	}
}

func TestDiagnosticCaptureBudgetBoundsRetainedFiles(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	config := testConfig()
	config.EnableTransportDiagnostics = true
	config.TransportDiagnosticDirectory = directory
	config.TransportDiagnosticFileLimit = 1
	config.TransportDiagnosticByteLimit = minimumDiagnosticBytes
	budget, err := newDiagnosticCaptureBudget(directory, config, &Metrics{})
	if err != nil {
		t.Fatalf("create diagnostic budget: %v", err)
	}
	if !budget.reserveFile() {
		t.Fatal("first diagnostic file rejected")
	}
	if budget.reserveFile() {
		t.Fatal("diagnostic file limit exceeded")
	}
}
