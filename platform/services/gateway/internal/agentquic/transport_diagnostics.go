package agentquic

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlog"
	"github.com/quic-go/quic-go/qlogwriter"
)

const (
	diagnosticDirectoryMode = 0o700
	diagnosticFileMode      = 0o600
	diagnosticBufferBytes   = 64 << 10
)

func newTransportDiagnosticTracer(config Config, metrics *Metrics) (
	func(context.Context, bool, quic.ConnectionID) qlogwriter.Trace,
	error,
) {
	if !config.EnableTransportDiagnostics {
		return nil, nil
	}

	directory := filepath.Clean(config.TransportDiagnosticDirectory)
	if err := os.MkdirAll(directory, diagnosticDirectoryMode); err != nil {
		return nil, fmt.Errorf("create QUIC diagnostic directory: %w", err)
	}

	info, err := os.Stat(directory)
	if err != nil {
		return nil, fmt.Errorf("inspect QUIC diagnostic directory: %w", err)
	}

	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("inspect QUIC diagnostic directory: owner-only directory required")
	}

	budget, err := newDiagnosticCaptureBudget(directory, config, metrics)
	if err != nil {
		return nil, err
	}

	return func(_ context.Context, isClient bool, connectionID quic.ConnectionID) qlogwriter.Trace {
		if !budget.reserveFile() {
			slog.Warn("QUIC diagnostic file limit reached")
			return nil
		}

		path := filepath.Join(directory, connectionID.String()+"_server.sqlog")

		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, diagnosticFileMode) // #nosec G304 -- directory is validated configuration and filename is a connection ID.
		if err != nil {
			budget.releaseFile()
			slog.Warn("QUIC diagnostic trace unavailable", "error", err)

			return nil
		}

		metrics.qlogSessions.Add(1)
		writer := &diagnosticBufferedWriter{
			buffer: bufio.NewWriterSize(&boundedDiagnosticWriter{writer: file, budget: budget, metrics: metrics}, diagnosticBufferBytes),
			file:   file,
		}

		trace := qlogwriter.NewConnectionFileSeq(writer, isClient, connectionID, []string{qlog.EventSchema})
		go trace.Run()

		return trace
	}, nil
}

type diagnosticCaptureBudget struct {
	remainingFiles atomic.Uint64
	remainingBytes atomic.Uint64
}

func newDiagnosticCaptureBudget(directory string, config Config, metrics *Metrics) (*diagnosticCaptureBudget, error) {
	retainedFiles, retainedBytes, err := inspectRetainedDiagnostics(directory)
	if err != nil {
		return nil, err
	}

	if retainedFiles > uint64(config.TransportDiagnosticFileLimit) || retainedBytes > config.TransportDiagnosticByteLimit {
		return nil, fmt.Errorf("inspect QUIC diagnostic retention: configured bound already exceeded")
	}

	budget := &diagnosticCaptureBudget{}
	budget.remainingFiles.Store(uint64(config.TransportDiagnosticFileLimit) - retainedFiles)
	budget.remainingBytes.Store(config.TransportDiagnosticByteLimit - retainedBytes)
	metrics.qlogBytes.Add(retainedBytes)

	return budget, nil
}

func inspectRetainedDiagnostics(directory string) (uint64, uint64, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, 0, fmt.Errorf("inspect QUIC diagnostic retention: %w", err)
	}

	var retainedFiles uint64

	var retainedBytes uint64

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), "_server.sqlog") {
			continue
		}

		if entry.Type()&os.ModeSymlink != 0 {
			return 0, 0, fmt.Errorf("inspect QUIC diagnostic retention: symbolic links are not allowed")
		}

		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
			return 0, 0, fmt.Errorf("inspect QUIC diagnostic retention: invalid trace file")
		}

		retainedFiles++

		fileBytes, err := uint64FromNonnegativeInt64(info.Size(), "diagnostic file size")
		if err != nil {
			return 0, 0, err
		}

		if retainedBytes > ^uint64(0)-fileBytes {
			return 0, 0, fmt.Errorf("inspect QUIC diagnostic retention: byte count overflow")
		}

		retainedBytes += fileBytes
	}

	return retainedFiles, retainedBytes, nil
}

func (budget *diagnosticCaptureBudget) reserveFile() bool {
	for {
		remaining := budget.remainingFiles.Load()
		if remaining == 0 {
			return false
		}

		if budget.remainingFiles.CompareAndSwap(remaining, remaining-1) {
			return true
		}
	}
}

func (budget *diagnosticCaptureBudget) releaseFile() {
	budget.remainingFiles.Add(1)
}

func (budget *diagnosticCaptureBudget) reserveBytes(requested uint64) uint64 {
	for {
		remaining := budget.remainingBytes.Load()

		reserved := min(requested, remaining)
		if reserved == 0 {
			return 0
		}

		if budget.remainingBytes.CompareAndSwap(remaining, remaining-reserved) {
			return reserved
		}
	}
}

type boundedDiagnosticWriter struct {
	writer  io.Writer
	budget  *diagnosticCaptureBudget
	metrics *Metrics
}

func (writer *boundedDiagnosticWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	requested, conversionErr := uint64FromNonnegativeInt(len(data), "diagnostic write length")
	if conversionErr != nil {
		return 0, conversionErr
	}

	reserved := writer.budget.reserveBytes(requested)
	if reserved == 0 {
		return 0, io.ErrShortWrite
	}

	written, err := writer.writer.Write(data[:reserved])

	writtenBytes, conversionErr := uint64FromNonnegativeInt(written, "diagnostic bytes written")
	if conversionErr != nil || writtenBytes > reserved {
		writer.budget.remainingBytes.Add(reserved)
		return 0, fmt.Errorf("write bounded QUIC diagnostic: invalid writer count")
	}

	if unused := reserved - writtenBytes; unused > 0 {
		writer.budget.remainingBytes.Add(unused)
	}

	if written > 0 {
		writer.metrics.qlogBytes.Add(writtenBytes)
	}

	if err == nil && written < len(data) {
		err = io.ErrShortWrite
	}

	return written, err
}

type diagnosticBufferedWriter struct {
	buffer *bufio.Writer
	file   *os.File
}

func (writer *diagnosticBufferedWriter) Write(data []byte) (int, error) {
	return writer.buffer.Write(data)
}

func (writer *diagnosticBufferedWriter) Close() error {
	flushError := writer.buffer.Flush()
	closeError := writer.file.Close()

	if flushError != nil {
		return flushError
	}

	return closeError
}
