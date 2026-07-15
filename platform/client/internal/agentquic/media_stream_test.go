package agentquic

import (
	"bytes"
	"testing"
	"time"
)

func TestMediaStreamPendingFrameUsesLatestSubmission(t *testing.T) {
	t.Parallel()

	stream := &MediaStream{
		maximumBytes: 16,
		pendingFrame: make(chan mediaFrameSubmission, 1),
	}
	first := []byte("first")
	second := []byte("second")
	if err := stream.WriteJPEG(first, time.UnixMilli(1)); err != nil {
		t.Fatalf("submit first frame: %v", err)
	}
	if err := stream.WriteJPEG(second, time.UnixMilli(2)); err != nil {
		t.Fatalf("submit replacement frame: %v", err)
	}
	first[0] = 'X'
	second[0] = 'Y'

	submission := <-stream.pendingFrame
	if !bytes.Equal(submission.data, []byte("second")) {
		t.Fatalf("pending frame = %q, want latest submission", submission.data)
	}
	if submission.capturedAt.UnixMilli() != 2 {
		t.Fatalf("capture time = %d, want 2", submission.capturedAt.UnixMilli())
	}
	if dropped := stream.StaleFramesDropped(); dropped != 1 {
		t.Fatalf("stale drops = %d, want 1", dropped)
	}
}

func TestMediaStreamRejectsInvalidSubmission(t *testing.T) {
	t.Parallel()

	stream := &MediaStream{
		maximumBytes: 1,
		pendingFrame: make(chan mediaFrameSubmission, 1),
	}
	if err := stream.WriteJPEG(nil, time.Now()); err == nil {
		t.Fatal("empty frame accepted")
	}
	if err := stream.WriteJPEG([]byte{1, 2}, time.Now()); err == nil {
		t.Fatal("oversized frame accepted")
	}
	stream.closed = true
	if err := stream.WriteJPEG([]byte{1}, time.Now()); err == nil {
		t.Fatal("closed stream accepted frame")
	}
}

func TestMediaWriteTimeoutIsBounded(t *testing.T) {
	t.Parallel()

	if timeout, err := mediaWriteTimeout(120); err != nil || timeout != minimumMediaWriteTimeout {
		t.Fatalf("high-rate timeout = %s, want %s", timeout, minimumMediaWriteTimeout)
	}
	if timeout, err := mediaWriteTimeout(1); err != nil || timeout != 2*time.Second {
		t.Fatalf("low-rate timeout = %s, want 2s", timeout)
	}
}
