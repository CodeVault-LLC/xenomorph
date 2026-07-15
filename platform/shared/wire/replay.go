package wire

import (
	"fmt"
	"sync"
)

const replayWordBits = 64

// ReplayWindow accepts out-of-order unique sequences within a fixed bitmap.
// It is safe for concurrent streams in one authenticated session.
type ReplayWindow struct {
	mu         sync.Mutex
	words      []uint64
	width      uint64
	maximumGap uint64
	highest    uint64
}

// NewReplayWindow creates a fixed-size sliding window and forward-gap bound.
func NewReplayWindow(width, maximumGap uint64) (*ReplayWindow, error) {
	if width == 0 || width%replayWordBits != 0 || maximumGap < width {
		return nil, fmt.Errorf("create XBP replay window: %w: invalid width or gap", ErrLimit)
	}

	return &ReplayWindow{
		words:      make([]uint64, width/replayWordBits),
		width:      width,
		maximumGap: maximumGap,
	}, nil
}

// Accept records a sequence or rejects zero, duplicates, stale values, and
// forward jumps beyond the configured gap without allocating new storage.
func (window *ReplayWindow) Accept(sequence uint64) error {
	if window == nil || sequence == 0 {
		return fmt.Errorf("accept XBP sequence: %w: zero sequence", ErrReplay)
	}

	window.mu.Lock()
	defer window.mu.Unlock()

	if window.highest == 0 {
		window.highest = sequence
		window.set(sequence)

		return nil
	}

	if sequence > window.highest {
		gap := sequence - window.highest
		if gap > window.maximumGap {
			return fmt.Errorf("accept XBP sequence: %w: forward gap %d", ErrReplay, gap)
		}

		window.advance(gap)
		window.highest = sequence
		window.set(sequence)

		return nil
	}

	if window.highest-sequence >= window.width {
		return fmt.Errorf("accept XBP sequence: %w: stale sequence", ErrReplay)
	}

	if window.seen(sequence) {
		return fmt.Errorf("accept XBP sequence: %w: duplicate sequence", ErrReplay)
	}

	window.set(sequence)

	return nil
}

func (window *ReplayWindow) advance(gap uint64) {
	if gap >= window.width {
		clear(window.words)
		return
	}

	for sequence := window.highest + 1; sequence <= window.highest+gap; sequence++ {
		window.clear(sequence)
	}
}

func (window *ReplayWindow) seen(sequence uint64) bool {
	word, bit := window.position(sequence)
	return window.words[word]&(uint64(1)<<bit) != 0
}

func (window *ReplayWindow) set(sequence uint64) {
	word, bit := window.position(sequence)
	window.words[word] |= uint64(1) << bit
}

func (window *ReplayWindow) clear(sequence uint64) {
	word, bit := window.position(sequence)
	window.words[word] &^= uint64(1) << bit
}

func (window *ReplayWindow) position(sequence uint64) (uint64, uint64) {
	position := sequence % window.width
	return position / replayWordBits, position % replayWordBits
}
