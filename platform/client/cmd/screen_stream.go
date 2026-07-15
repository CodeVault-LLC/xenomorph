package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
	"github.com/codevault-llc/xenomorph/platform/client/internal/agentquic"
)

const (
	defaultScreenStreamFPS int           = 30
	maxScreenStreamFPS     int           = 60
	defaultJPEGQuality     int           = 70
	maxJPEGQuality         int           = 100
	reconnectDelay         time.Duration = 500 * time.Millisecond
	mediaOpenTimeout       time.Duration = 5 * time.Second
)

type screenStreamPayload struct {
	FPS               int    `json:"fps"`
	Quality           int    `json:"quality"`
	GenerationID      string `json:"generation_id"`
	MaximumFrameBytes uint64 `json:"maximum_frame_bytes"`
}

type screenStreamer struct {
	quicClient *agentquic.Client

	mu     sync.Mutex
	cancel context.CancelFunc
}

func newScreenStreamer(quicClient *agentquic.Client) *screenStreamer {
	return &screenStreamer{quicClient: quicClient}
}

func (s *screenStreamer) Start(payload json.RawMessage) error {
	if s == nil {
		return nil
	}

	config := screenStreamPayload{FPS: defaultScreenStreamFPS}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &config); err != nil {
			return fmt.Errorf("decode screen stream payload: %w", err)
		}
	}

	fps := clampScreenFPS(config.FPS)
	quality := clampJPEGQuality(config.Quality)

	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	go s.run(ctx, config, fps, quality)

	return nil
}

func (s *screenStreamer) Stop() {
	if s == nil {
		return
	}

	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.mu.Unlock()
}

func (s *screenStreamer) run(ctx context.Context, config screenStreamPayload, fps int, quality int) {
	s.runQUIC(ctx, config, fps, quality)
}

func (s *screenStreamer) runQUIC(ctx context.Context, config screenStreamPayload, fps int, quality int) {
	contract, err := mediaGenerationContract(config, fps)
	if err != nil {
		return
	}

	for ctx.Err() == nil {
		firstFrame, captureErr := captureScreenJPEGFrame(quality)
		if captureErr != nil {
			waitBeforeReconnect(ctx)
			continue
		}

		options, optionsErr := mediaOptionsForFrame(contract, firstFrame)
		if optionsErr != nil {
			return
		}

		openContext, cancel := context.WithTimeout(ctx, mediaOpenTimeout)
		media, openErr := s.quicClient.OpenMediaStream(openContext, options)

		cancel()

		if openErr != nil {
			waitBeforeReconnect(ctx)
			continue
		}

		if media.WriteJPEG(firstFrame.data, time.Now().UTC()) == nil {
			s.writeQUICFrames(ctx, media, fps, quality)
		}

		_ = media.Close()

		waitBeforeReconnect(ctx)
	}
}

func mediaGenerationContract(config screenStreamPayload, fps int) (agentquic.MediaStreamOptions, error) {
	generation, err := uuid.Parse(config.GenerationID)
	if err != nil || config.MaximumFrameBytes == 0 || config.MaximumFrameBytes > 10<<20 {
		return agentquic.MediaStreamOptions{}, fmt.Errorf("invalid signed media generation contract")
	}

	frameRateCap, err := positiveDimension(fps)
	if err != nil {
		return agentquic.MediaStreamOptions{}, err
	}

	var generationID [16]byte

	copy(generationID[:], generation[:])

	return agentquic.MediaStreamOptions{
		GenerationID: generationID, FrameRateCap: frameRateCap,
		MaximumFrameBytes: config.MaximumFrameBytes,
	}, nil
}

func mediaOptionsForFrame(contract agentquic.MediaStreamOptions, frame encodedScreenFrame) (agentquic.MediaStreamOptions, error) {
	width, err := positiveDimension(frame.width)
	if err != nil {
		return agentquic.MediaStreamOptions{}, err
	}

	height, err := positiveDimension(frame.height)
	if err != nil {
		return agentquic.MediaStreamOptions{}, err
	}

	contract.Width = width
	contract.Height = height

	return contract, nil
}

func positiveDimension(value int) (uint64, error) {
	if value <= 0 {
		return 0, fmt.Errorf("screen dimension must be positive")
	}

	return uint64(value), nil
}

func (s *screenStreamer) writeQUICFrames(ctx context.Context, media *agentquic.MediaStream, fps, quality int) {
	ticker := time.NewTicker(time.Second / time.Duration(fps))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case capturedAt := <-ticker.C:
			frame, err := captureScreenJPEGFrame(quality)
			if err != nil {
				continue
			}

			if err := media.WriteJPEG(frame.data, capturedAt); err != nil {
				return
			}
		}
	}
}

func clampScreenFPS(value int) int {
	if value <= 0 {
		return defaultScreenStreamFPS
	}

	if value > maxScreenStreamFPS {
		return maxScreenStreamFPS
	}

	return value
}

func clampJPEGQuality(value int) int {
	if value <= 0 {
		return defaultJPEGQuality
	}

	if value > maxJPEGQuality {
		return maxJPEGQuality
	}

	return value
}

type encodedScreenFrame struct {
	data   []byte
	width  int
	height int
}

func captureScreenJPEGFrame(quality int) (encodedScreenFrame, error) {
	data, err := agent.CaptureScreenshot()
	if err != nil {
		return encodedScreenFrame{}, err
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return encodedScreenFrame{}, fmt.Errorf("decode screenshot: %w", err)
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}); err != nil {
		return encodedScreenFrame{}, fmt.Errorf("encode jpeg: %w", err)
	}

	bounds := img.Bounds()

	return encodedScreenFrame{data: out.Bytes(), width: bounds.Dx(), height: bounds.Dy()}, nil
}

func waitBeforeReconnect(ctx context.Context) {
	timer := time.NewTimer(reconnectDelay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
