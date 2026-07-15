package agentquic

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const (
	minimumMediaWriteTimeout  = time.Second
	maximumMediaWriteTimeout  = 5 * time.Second
	maximumMediaDimension     = uint64(16_384)
	maximumMediaFrameRate     = uint64(120)
	mediaFrameDeadlinePeriods = int64(2)
)

// MediaStreamOptions is the signed-command-authored screen generation contract.
type MediaStreamOptions struct {
	GenerationID      [16]byte
	Width             uint64
	Height            uint64
	FrameRateCap      uint64
	MaximumFrameBytes uint64
}

// MediaStream owns one reliable, ordered screen generation stream.
type MediaStream struct {
	session      *clientSession
	stream       *quic.SendStream
	codec        wire.FrameCodec
	generationID [16]byte
	maximumBytes uint64
	writeTimeout time.Duration
	pendingFrame chan mediaFrameSubmission
	writerDone   chan struct{}

	mu          sync.Mutex
	closed      bool
	writerError error

	staleFramesDropped atomic.Uint64
}

type mediaFrameSubmission struct {
	data       []byte
	capturedAt time.Time
}

// OpenMediaStream validates and opens one JPEG media generation.
func (client *Client) OpenMediaStream(ctx context.Context, options MediaStreamOptions) (*MediaStream, error) {
	if err := validateMediaStreamOptions(options); err != nil {
		return nil, err
	}
	session, err := client.waitSession(ctx)
	if err != nil {
		return nil, err
	}
	stream, codec, err := openMediaLane(ctx, session)
	if err != nil {
		return nil, err
	}
	if err := authorizeMediaLane(ctx, session, stream, codec, options); err != nil {
		_ = stream.Close()
		return nil, err
	}
	writeTimeout, err := mediaWriteTimeout(options.FrameRateCap)
	if err != nil {
		_ = stream.Close()
		return nil, err
	}
	mediaStream := &MediaStream{
		session: session, stream: stream, codec: codec,
		generationID: options.GenerationID, maximumBytes: options.MaximumFrameBytes,
		writeTimeout: writeTimeout,
		pendingFrame: make(chan mediaFrameSubmission, 1), writerDone: make(chan struct{}),
	}
	go mediaStream.runWriter()
	return mediaStream, nil
}

func mediaWriteTimeout(frameRateCap uint64) (time.Duration, error) {
	frameRate, err := int64FromUint64(frameRateCap, "media frame rate")
	if err != nil || frameRate <= 0 {
		return 0, fmt.Errorf("calculate QUIC media write timeout: invalid frame rate")
	}
	frameInterval := time.Second / time.Duration(frameRate)
	timeout := min(max(time.Duration(mediaFrameDeadlinePeriods)*frameInterval, minimumMediaWriteTimeout), maximumMediaWriteTimeout)
	return timeout, nil
}

func validateMediaStreamOptions(options MediaStreamOptions) error {
	if options.GenerationID == [16]byte{} || options.Width == 0 || options.Height == 0 ||
		options.Width > maximumMediaDimension || options.Height > maximumMediaDimension ||
		options.FrameRateCap == 0 || options.FrameRateCap > maximumMediaFrameRate ||
		options.MaximumFrameBytes == 0 || options.MaximumFrameBytes > 10<<20 {
		return fmt.Errorf("open QUIC media stream: invalid signed media contract")
	}
	return nil
}

func openMediaLane(ctx context.Context, session *clientSession) (*quic.SendStream, wire.FrameCodec, error) {
	stream, err := session.connection.OpenUniStreamSync(ctx)
	if err != nil {
		return nil, wire.FrameCodec{}, fmt.Errorf("open QUIC media stream: %w", err)
	}
	if err := wire.WritePreamble(stream, wire.StreamMedia); err != nil {
		return nil, wire.FrameCodec{}, err
	}
	specification, _ := wire.SpecificationForStream(wire.StreamMedia)
	codec, err := wire.NewFrameCodec(specification.MaximumFrameBytes)
	return stream, codec, err
}

func authorizeMediaLane(
	ctx context.Context,
	session *clientSession,
	stream *quic.SendStream,
	codec wire.FrameCodec,
	options MediaStreamOptions,
) error {
	body, err := (wire.MediaOpen{
		GenerationID: options.GenerationID, Codec: 1, Width: options.Width, Height: options.Height,
		FrameRateCap: options.FrameRateCap, MaximumFrameBytes: options.MaximumFrameBytes,
	}).MarshalBinary()
	if err != nil {
		return err
	}
	sequence := session.nextSequence()
	response := make(chan eventResponse, 1)
	session.registerPending(sequence, response)
	if err := codec.WriteFrame(stream, wire.Frame{
		Header: wire.FrameHeader{
			Type: wire.MessageMediaOpen, SchemaRevision: 1,
			Flags: wire.FlagAckRequired, Sequence: sequence,
		},
		Body: body,
	}); err != nil {
		session.completePending(sequence, eventResponse{err: err})
		return fmt.Errorf("write QUIC media open: %w", err)
	}
	select {
	case acknowledgement := <-response:
		if acknowledgement.err != nil {
			return acknowledgement.err
		}
	case <-ctx.Done():
		session.completePending(sequence, eventResponse{err: ctx.Err()})
		return fmt.Errorf("wait for QUIC media authorization: %w", ctx.Err())
	}
	return nil
}

// WriteJPEG submits the latest encoded frame without blocking on network I/O.
// A generation retains at most one frame waiting behind the active socket
// write. A newer submission replaces that stale pending frame.
func (stream *MediaStream) WriteJPEG(data []byte, capturedAt time.Time) error {
	if len(data) == 0 || uint64(len(data)) > stream.maximumBytes {
		return fmt.Errorf("write QUIC media frame: encoded frame exceeds generation limit")
	}
	submission := mediaFrameSubmission{
		data: append([]byte(nil), data...), capturedAt: capturedAt.UTC(),
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.closed {
		return fmt.Errorf("write QUIC media frame: stream is closed")
	}
	if stream.writerError != nil {
		return stream.writerError
	}
	select {
	case stream.pendingFrame <- submission:
		return nil
	default:
	}
	select {
	case <-stream.pendingFrame:
		stream.staleFramesDropped.Add(1)
	default:
	}
	stream.pendingFrame <- submission
	return nil
}

// StaleFramesDropped returns the number of pending frames replaced before a
// network write began.
func (stream *MediaStream) StaleFramesDropped() uint64 {
	if stream == nil {
		return 0
	}
	return stream.staleFramesDropped.Load()
}

func (stream *MediaStream) runWriter() {
	var writerError error
	defer func() {
		stream.mu.Lock()
		stream.writerError = writerError
		stream.mu.Unlock()
		close(stream.writerDone)
	}()

	var frameNumber uint64
	for submission := range stream.pendingFrame {
		frameNumber++
		if err := stream.writeFrame(submission, frameNumber); err != nil {
			writerError = err
			_ = stream.stream.Close()
			return
		}
	}
	writerError = stream.writeClose(frameNumber)
}

func (stream *MediaStream) writeFrame(submission mediaFrameSubmission, frameNumber uint64) error {
	if err := stream.stream.SetWriteDeadline(time.Now().Add(stream.writeTimeout)); err != nil {
		return fmt.Errorf("set QUIC media write deadline: %w", err)
	}
	capturedAt, err := unixMilliseconds(submission.capturedAt, "media capture time")
	if err != nil {
		return err
	}
	body, err := (wire.MediaFrame{
		GenerationID: stream.generationID, FrameNumber: frameNumber,
		CapturedAtMilliseconds: capturedAt, Keyframe: true,
		ContentType: 1, Data: submission.data,
	}).MarshalBinary()
	if err != nil {
		return fmt.Errorf("encode QUIC media frame: %w", err)
	}
	if err := stream.codec.WriteFrame(stream.stream, wire.Frame{
		Header: wire.FrameHeader{
			Type: wire.MessageMediaFrame, SchemaRevision: 1,
			Flags: wire.FlagSensitive, Sequence: stream.session.nextSequence(),
		},
		Body: body,
	}); err != nil {
		return fmt.Errorf("write QUIC media frame: %w", err)
	}
	return nil
}

// Close drains the single pending frame and ends the current media generation.
func (stream *MediaStream) Close() error {
	stream.mu.Lock()
	if stream.closed {
		writerDone := stream.writerDone
		stream.mu.Unlock()
		<-writerDone
		return stream.finalWriterError()
	}
	stream.closed = true
	close(stream.pendingFrame)
	writerDone := stream.writerDone
	stream.mu.Unlock()
	<-writerDone
	return stream.finalWriterError()
}

func (stream *MediaStream) finalWriterError() error {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return stream.writerError
}

func (stream *MediaStream) writeClose(frameNumber uint64) error {
	if err := stream.stream.SetWriteDeadline(time.Now().Add(stream.writeTimeout)); err != nil {
		return fmt.Errorf("set QUIC media close deadline: %w", err)
	}
	body, err := (wire.MediaClose{Reason: 1, LastFrameNumber: frameNumber}).MarshalBinary()
	if err != nil {
		return err
	}
	writeError := stream.codec.WriteFrame(stream.stream, wire.Frame{
		Header: wire.FrameHeader{
			Type: wire.MessageMediaClose, SchemaRevision: 1,
			Flags:    wire.FlagSensitive | wire.FlagEndOperation,
			Sequence: stream.session.nextSequence(),
		},
		Body: body,
	})
	closeError := stream.stream.Close()
	if writeError != nil {
		return fmt.Errorf("write QUIC media close: %w", writeError)
	}
	return closeError
}
