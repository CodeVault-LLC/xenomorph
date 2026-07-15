package wire

import (
	"errors"
	"testing"
)

func TestReplayWindowAcceptsBoundedReordering(t *testing.T) {
	t.Parallel()

	window, err := NewReplayWindow(64, 128)
	if err != nil {
		t.Fatalf("create replay window: %v", err)
	}

	for _, sequence := range []uint64{1, 3, 2, 66, 65, 64} {
		if err := window.Accept(sequence); err != nil {
			t.Fatalf("accept sequence %d: %v", sequence, err)
		}
	}

	if err := window.Accept(65); err == nil || !errors.Is(err, ErrReplay) {
		t.Fatalf("duplicate error = %v, want ErrReplay", err)
	}

	if err := window.Accept(1); err == nil || !errors.Is(err, ErrReplay) {
		t.Fatalf("stale error = %v, want ErrReplay", err)
	}

	if err := window.Accept(1024); err == nil || !errors.Is(err, ErrReplay) {
		t.Fatalf("gap error = %v, want ErrReplay", err)
	}
}
