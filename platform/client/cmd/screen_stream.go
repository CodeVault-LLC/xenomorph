package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

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
	gatewayURL string
	tlsConfig  *tls.Config
	quicClient *agentquic.Client

	mu     sync.Mutex
	cancel context.CancelFunc
}

func newScreenStreamer(gatewayURL string, tlsConfig *tls.Config) *screenStreamer {
	return &screenStreamer{
		gatewayURL: gatewayURL,
		tlsConfig:  tlsConfig,
	}
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
	if s.quicClient != nil {
		s.runQUIC(ctx, config, fps, quality)
		return
	}

	for ctx.Err() == nil {
		conn, err := s.dial(ctx)
		if err != nil {
			waitBeforeReconnect(ctx)
			continue
		}

		s.writeFrames(ctx, conn, fps, quality)
		_ = conn.Close()

		waitBeforeReconnect(ctx)
	}
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

func (s *screenStreamer) writeFrames(ctx context.Context, conn *websocket.Conn, fps int, quality int) {
	ticker := time.NewTicker(time.Second / time.Duration(fps))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			frame, err := captureScreenJPEG(quality)
			if err != nil {
				continue
			}

			if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				return
			}
		}
	}
}

func (s *screenStreamer) dial(ctx context.Context) (*websocket.Conn, error) {
	u, err := url.Parse(s.gatewayURL)
	if err != nil {
		return nil, fmt.Errorf("parse gateway url: %w", err)
	}

	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return nil, fmt.Errorf("unsupported gateway scheme %q", u.Scheme)
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/screen/media"
	q := u.Query()
	q.Set("content_type", "image/jpeg")
	u.RawQuery = q.Encode()

	dialer := websocket.Dialer{TLSClientConfig: s.tlsConfig}

	conn, resp, err := dialer.DialContext(ctx, u.String(), nil)
	if resp != nil {
		_ = resp.Body.Close()
	}

	return conn, err
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

func captureScreenJPEG(quality int) ([]byte, error) {
	frame, err := captureScreenJPEGFrame(quality)
	return frame.data, err
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
