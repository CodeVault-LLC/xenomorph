package agentquic

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const (
	maximumRemoteAddressBytes  = 256
	heartbeatLivenessIntervals = 2
)

func (current *session) acceptUnidirectionalStreams(ctx context.Context) error {
	workers := make(chan struct{}, int(current.listener.config.MaximumIncomingUniStreams))

	var operationWaiter operationWaitGroup
	defer operationWaiter.wait()

	for {
		stream, err := current.connection.AcceptUniStream(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf("accept unidirectional stream: %w", err)
		}
		select {
		case workers <- struct{}{}:
			operationWaiter.start(func() {
				defer func() { <-workers }()

				if err := current.classifyUnidirectionalStream(ctx, stream); err != nil && ctx.Err() == nil {
					stream.CancelRead(quic.StreamErrorCode(errorCode(err)))
				}
			})
		default:
			stream.CancelRead(quic.StreamErrorCode(wire.ApplicationLimit))
		}
	}
}

func (current *session) acceptBidirectionalStreams(ctx context.Context) error {
	workers := make(chan struct{}, int(current.listener.config.MaximumIncomingStreams))

	var operationWaiter operationWaitGroup
	defer operationWaiter.wait()

	for {
		stream, err := current.connection.AcceptStream(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf("accept bidirectional stream: %w", err)
		}
		select {
		case workers <- struct{}{}:
			operationWaiter.start(func() {
				defer func() { <-workers }()

				if err := current.classifyBidirectionalStream(ctx, stream); err != nil && ctx.Err() == nil {
					stream.CancelRead(quic.StreamErrorCode(errorCode(err)))
					stream.CancelWrite(quic.StreamErrorCode(errorCode(err)))
				}
			})
		default:
			stream.CancelRead(quic.StreamErrorCode(wire.ApplicationLimit))
			stream.CancelWrite(quic.StreamErrorCode(wire.ApplicationLimit))
		}
	}
}

func (current *session) classifyUnidirectionalStream(ctx context.Context, stream *quic.ReceiveStream) error {
	streamID, err := streamIDValue(int64(stream.StreamID()))
	if err != nil || streamID&0x3 != 0x2 {
		return fmt.Errorf("classify unidirectional stream: %w: wrong initiator or direction", wire.ErrUnexpectedMessage)
	}

	if err := stream.SetReadDeadline(time.Now().Add(current.listener.config.ControlStreamTimeout)); err != nil {
		return err
	}

	preamble, err := wire.ReadPreamble(stream)
	if err != nil {
		return err
	}

	if err := stream.SetReadDeadline(time.Time{}); err != nil {
		return err
	}

	if preamble.Kind == wire.StreamEvents {
		return current.claimEventStream(ctx, stream)
	}

	if preamble.Kind == wire.StreamMedia {
		return current.claimMediaStream(ctx, stream)
	}

	return fmt.Errorf("classify unidirectional stream: %w: stream kind %d", wire.ErrUnexpectedMessage, preamble.Kind)
}

func (current *session) claimEventStream(ctx context.Context, stream *quic.ReceiveStream) error {
	if !current.eventClaimed.CompareAndSwap(false, true) {
		return fmt.Errorf("classify event stream: %w: duplicate mandatory lane", wire.ErrUnexpectedMessage)
	}

	return current.readEventStream(ctx, stream)
}

func (current *session) claimMediaStream(ctx context.Context, stream *quic.ReceiveStream) error {
	select {
	case current.mediaWorkers <- struct{}{}:
		defer func() { <-current.mediaWorkers }()
		return current.readMediaStream(ctx, stream)
	default:
		return fmt.Errorf("classify media stream: %w: media generation already active", wire.ErrLimit)
	}
}

func (current *session) classifyBidirectionalStream(ctx context.Context, stream *quic.Stream) error {
	streamID, err := streamIDValue(int64(stream.StreamID()))
	if err != nil || streamID&0x3 != 0 {
		return fmt.Errorf("classify bidirectional stream: %w: wrong initiator or direction", wire.ErrUnexpectedMessage)
	}

	if err := stream.SetReadDeadline(time.Now().Add(current.listener.config.ControlStreamTimeout)); err != nil {
		return err
	}

	preamble, err := wire.ReadPreamble(stream)
	if err != nil {
		return err
	}

	if err := stream.SetReadDeadline(time.Time{}); err != nil {
		return err
	}

	if preamble.Kind != wire.StreamTransfer {
		return fmt.Errorf("classify bidirectional stream: %w: stream kind %d", wire.ErrUnexpectedMessage, preamble.Kind)
	}
	select {
	case current.transferWorker <- struct{}{}:
		defer func() { <-current.transferWorker }()
		return current.readTransferStream(ctx, stream)
	default:
		return fmt.Errorf("classify transfer stream: %w: transfer concurrency reached", wire.ErrLimit)
	}
}

func (current *session) readEventStream(ctx context.Context, stream *quic.ReceiveStream) error {
	maximum := current.listener.config.EventFrameMaximum
	specification, _ := wire.SpecificationForStream(wire.StreamEvents)

	if maximum > specification.MaximumFrameBytes {
		maximum = specification.MaximumFrameBytes
	}

	codec, err := wire.NewFrameCodec(maximum)
	if err != nil {
		return err
	}

	for {
		if err := stream.SetReadDeadline(time.Now().Add(heartbeatLivenessIntervals * current.listener.config.HeartbeatInterval)); err != nil {
			return err
		}

		frame, err := codec.ReadFrame(stream)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return err
		}

		if err := current.dispatchIngressFrame(ctx, wire.StreamEvents, frame); err != nil {
			return err
		}
	}
}

func (current *session) readMediaStream(ctx context.Context, stream *quic.ReceiveStream) error {
	specification, _ := wire.SpecificationForStream(wire.StreamMedia)

	codec, err := wire.NewFrameCodec(specification.MaximumFrameBytes)
	if err != nil {
		return err
	}

	state := mediaStreamState{}

	for {
		readTimeout := current.listener.config.ControlStreamTimeout
		if state.opened {
			readTimeout = current.listener.config.MediaFrameTimeout
		}

		if err := stream.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			return err
		}

		frame, err := codec.ReadFrame(stream)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return err
		}

		if err := state.accept(frame); err != nil {
			return err
		}

		if err := current.dispatchIngressFrame(ctx, wire.StreamMedia, frame); err != nil {
			return err
		}

		if frame.Header.Type == wire.MessageMediaClose {
			return nil
		}
	}
}

type mediaStreamState struct {
	opened            bool
	generationID      [16]byte
	previousFrame     uint64
	maximumFrameBytes uint64
}

func (state *mediaStreamState) accept(frame wire.Frame) error {
	if (!state.opened && frame.Header.Type != wire.MessageMediaOpen) ||
		(state.opened && frame.Header.Type == wire.MessageMediaOpen) {
		return fmt.Errorf("read media stream: %w: invalid media state", wire.ErrUnexpectedMessage)
	}

	if frame.Header.Type == wire.MessageMediaOpen {
		return state.open(frame.Body)
	}

	if frame.Header.Type == wire.MessageMediaFrame {
		return state.frame(frame.Body)
	}

	state.opened = true

	return nil
}

func (state *mediaStreamState) open(body []byte) error {
	var open wire.MediaOpen
	if err := open.UnmarshalBinary(body); err != nil {
		return err
	}

	if open.GenerationID == [16]byte{} || open.Codec != 1 || open.Width == 0 || open.Height == 0 ||
		open.FrameRateCap == 0 || open.MaximumFrameBytes == 0 {
		return fmt.Errorf("read media stream: %w: invalid media contract", wire.ErrEncoding)
	}

	state.opened = true
	state.generationID = open.GenerationID
	state.maximumFrameBytes = open.MaximumFrameBytes

	return nil
}

func (state *mediaStreamState) frame(body []byte) error {
	var mediaFrame wire.MediaFrame
	if err := mediaFrame.UnmarshalBinary(body); err != nil {
		return err
	}

	if mediaFrame.GenerationID != state.generationID || mediaFrame.FrameNumber <= state.previousFrame ||
		uint64(len(mediaFrame.Data)) > state.maximumFrameBytes {
		return fmt.Errorf("read media stream: %w: media generation, order, or size mismatch", wire.ErrEncoding)
	}

	state.previousFrame = mediaFrame.FrameNumber

	return nil
}

func (current *session) readTransferStream(ctx context.Context, stream *quic.Stream) error {
	specification, _ := wire.SpecificationForStream(wire.StreamTransfer)

	codec, err := wire.NewFrameCodec(specification.MaximumFrameBytes)
	if err != nil {
		return err
	}

	openFrame, open, err := current.readTransferOpen(stream, codec)
	if err != nil {
		return err
	}

	if err := current.dispatchIngressFrame(ctx, wire.StreamTransfer, openFrame); err != nil {
		return err
	}

	return current.routeTransferDirection(ctx, stream, codec, openFrame, open)
}

func (current *session) readTransferOpen(stream *quic.Stream, codec wire.FrameCodec) (wire.Frame, wire.TransferOpen, error) {
	if err := stream.SetDeadline(time.Now().Add(current.listener.config.ControlStreamTimeout)); err != nil {
		return wire.Frame{}, wire.TransferOpen{}, err
	}

	openFrame, err := codec.ReadFrame(stream)
	if err != nil {
		return wire.Frame{}, wire.TransferOpen{}, err
	}

	if openFrame.Header.Type != wire.MessageTransferOpen || openFrame.Header.Flags&wire.FlagHasOperationID == 0 {
		return wire.Frame{}, wire.TransferOpen{}, fmt.Errorf("read transfer stream: %w: TransferOpen must be first", wire.ErrUnexpectedMessage)
	}

	var open wire.TransferOpen
	if err := open.UnmarshalBinary(openFrame.Body); err != nil {
		return wire.Frame{}, wire.TransferOpen{}, err
	}

	if open.TransferID != openFrame.Header.OperationID {
		return wire.Frame{}, wire.TransferOpen{}, fmt.Errorf("read transfer stream: %w: operation ID mismatch", wire.ErrEncoding)
	}

	return openFrame, open, nil
}

func (current *session) routeTransferDirection(
	ctx context.Context,
	stream *quic.Stream,
	codec wire.FrameCodec,
	openFrame wire.Frame,
	open wire.TransferOpen,
) error {
	if wire.TransferDirection(open.Direction) == wire.TransferGatewayToAgent {
		return current.writeGatewayTransfer(ctx, stream, codec, openFrame, open)
	}

	if wire.TransferDirection(open.Direction) != wire.TransferAgentToGateway {
		return fmt.Errorf("read transfer stream: %w: invalid direction", wire.ErrEncoding)
	}

	return current.readAgentTransfer(ctx, stream, codec, open.TransferID)
}

func (current *session) readAgentTransfer(
	ctx context.Context,
	stream *quic.Stream,
	codec wire.FrameCodec,
	operationID [16]byte,
) error {
	for {
		if err := current.refreshTransferStreamDeadline(stream); err != nil {
			return err
		}

		frame, err := codec.ReadFrame(stream)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return err
		}

		terminal, err := current.acceptAgentTransferFrame(ctx, stream, codec, operationID, frame)
		if err != nil {
			return err
		}

		if terminal {
			return stream.Close()
		}
	}
}

func (current *session) acceptAgentTransferFrame(
	ctx context.Context,
	stream *quic.Stream,
	codec wire.FrameCodec,
	operationID [16]byte,
	frame wire.Frame,
) (bool, error) {
	if frame.Header.OperationID != operationID || frame.Header.Flags&wire.FlagHasOperationID == 0 {
		return false, fmt.Errorf("read transfer stream: %w: operation changed", wire.ErrUnexpectedMessage)
	}

	if err := current.dispatchIngressFrame(ctx, wire.StreamTransfer, frame); err != nil {
		return false, err
	}

	if frame.Header.Type == wire.MessageTransferChunk {
		if err := current.writeTransferChunkAcknowledgement(stream, codec, frame); err != nil {
			return false, err
		}
	}

	terminal := frame.Header.Type == wire.MessageTransferFinalize || frame.Header.Type == wire.MessageTransferAbort

	return terminal, nil
}

func (current *session) writeGatewayTransfer(
	ctx context.Context,
	stream *quic.Stream,
	codec wire.FrameCodec,
	openFrame wire.Frame,
	open wire.TransferOpen,
) error {
	source, ok := current.listener.sink.(TransferChunkSource)
	if !ok {
		return fmt.Errorf("write gateway transfer: chunk source is unavailable")
	}

	receipt := current.receipt(openFrame)
	writer := gatewayTransferWriter{session: current, stream: stream, codec: codec, operationID: open.TransferID}

	err := source.StreamTransferChunks(ctx, receipt, open, writer.writeChunk)
	if err != nil {
		return err
	}

	return stream.Close()
}

type gatewayTransferWriter struct {
	session     *session
	stream      *quic.Stream
	codec       wire.FrameCodec
	operationID [16]byte
}

func (writer gatewayTransferWriter) writeChunk(chunk wire.TransferChunk) error {
	if err := writer.session.refreshTransferStreamDeadline(writer.stream); err != nil {
		return err
	}

	body, err := chunk.MarshalBinary()
	if err != nil {
		return err
	}

	sequence := writer.session.nextSequence()

	frame := wire.Frame{Header: wire.FrameHeader{
		Type: wire.MessageTransferChunk, SchemaRevision: 1,
		Flags:    wire.FlagAckRequired | wire.FlagHasOperationID | wire.FlagSensitive,
		Sequence: sequence, OperationID: writer.operationID,
	}, Body: body}
	if err := writer.codec.WriteFrame(writer.stream, frame); err != nil {
		return err
	}

	return writer.readAcknowledgement(sequence, chunk)
}

func (writer gatewayTransferWriter) readAcknowledgement(sequence uint64, chunk wire.TransferChunk) error {
	frame, err := writer.codec.ReadFrame(writer.stream)
	if err != nil {
		return err
	}

	if err := writer.session.replay.Accept(frame.Header.Sequence); err != nil {
		return err
	}

	if frame.Header.Type != wire.MessageTransferChunkAck || frame.Header.CorrelationSequence != sequence ||
		frame.Header.OperationID != writer.operationID {
		return wire.ErrUnexpectedMessage
	}

	var acknowledgement wire.TransferChunkAck
	if err := acknowledgement.UnmarshalBinary(frame.Body); err != nil {
		return err
	}

	if acknowledgement.ChunkIndex != chunk.ChunkIndex || acknowledgement.Digest != chunk.Digest {
		return fmt.Errorf("write gateway transfer: %w: chunk acknowledgement mismatch", wire.ErrEncoding)
	}

	return nil
}

func (current *session) writeTransferChunkAcknowledgement(
	stream *quic.Stream,
	codec wire.FrameCodec,
	frame wire.Frame,
) error {
	if err := current.refreshTransferStreamDeadline(stream); err != nil {
		return err
	}

	var chunk wire.TransferChunk
	if err := chunk.UnmarshalBinary(frame.Body); err != nil {
		return err
	}

	body, err := (wire.TransferChunkAck{ChunkIndex: chunk.ChunkIndex, Digest: chunk.Digest}).MarshalBinary()
	if err != nil {
		return err
	}

	acknowledgement := wire.Frame{
		Header: wire.FrameHeader{
			Type: wire.MessageTransferChunkAck, SchemaRevision: 1,
			Flags:    wire.FlagIsResponse | wire.FlagHasCorrelation | wire.FlagHasOperationID,
			Sequence: current.nextSequence(), CorrelationSequence: frame.Header.Sequence,
			OperationID: frame.Header.OperationID,
		}, Body: body,
	}
	if err := codec.WriteFrame(stream, acknowledgement); err != nil {
		return fmt.Errorf("write transfer chunk acknowledgement: %w", err)
	}

	return nil
}

func (current *session) refreshTransferStreamDeadline(stream *quic.Stream) error {
	if current == nil || current.listener == nil || stream == nil {
		return fmt.Errorf("set transfer stream deadline: invalid session or stream")
	}

	if err := stream.SetDeadline(time.Now().Add(current.listener.config.TransferStreamIOTimeout)); err != nil {
		return fmt.Errorf("set transfer stream deadline: %w", err)
	}

	return nil
}

func (current *session) dispatchIngressFrame(ctx context.Context, kind wire.StreamKind, frame wire.Frame) error {
	current.listener.metrics.decodedFrames.Add(1)

	if err := current.validateIngressFrame(kind, frame); err != nil {
		current.listener.metrics.rejectedFrames.Add(1)
		return err
	}

	message, err := decodeIngressMessage(frame)
	if err != nil {
		current.listener.metrics.rejectedFrames.Add(1)
		return err
	}

	result, commitErr := current.commitIngressMessage(ctx, frame, message)

	if frame.Header.Flags&wire.FlagAckRequired == 0 {
		return nil
	}

	if commitErr != nil {
		result = IngressResult{Status: wire.AcknowledgementFailed, Commit: wire.CommitValidated,
			Retry: wire.RetrySameOperation, PublicErrorCode: "commit_failed"}
	}

	return current.sendMessageAcknowledgement(ctx, frame.Header.Sequence, result)
}

func (current *session) validateIngressFrame(kind wire.StreamKind, frame wire.Frame) error {
	if err := current.replay.Accept(frame.Header.Sequence); err != nil {
		return err
	}

	if err := wire.ValidateMessageForStream(kind, frame.Header.Type, frame.Header.SchemaRevision); err != nil {
		return err
	}

	descriptor, _ := wire.DescriptorForMessage(frame.Header.Type)
	if descriptor.Acknowledgement == "required" && frame.Header.Flags&wire.FlagAckRequired == 0 {
		return fmt.Errorf("dispatch ingress frame: %w: required acknowledgement flag missing", wire.ErrEncoding)
	}

	if requiresOperationID(frame.Header.Type) && frame.Header.Flags&wire.FlagHasOperationID == 0 {
		return fmt.Errorf("dispatch ingress frame: %w: stable operation ID required", wire.ErrEncoding)
	}

	return nil
}

func (current *session) commitIngressMessage(ctx context.Context, frame wire.Frame, message IngressMessage) (IngressResult, error) {
	release, currentSession := current.listener.registry.beginCommit(current)
	if !currentSession {
		current.listener.metrics.fencedFrames.Add(1)
		return IngressResult{}, wire.ErrReplay
	}

	result, commitErr := current.listener.sink.CommitAgentMessage(ctx, current.receipt(frame), message)

	release()

	return result, commitErr
}

func (current *session) receipt(frame wire.Frame) IngressReceipt {
	remoteAddress := ""
	if current.connection.RemoteAddr() != nil {
		remoteAddress = current.connection.RemoteAddr().String()
	}

	if len(remoteAddress) > maximumRemoteAddressBytes {
		remoteAddress = remoteAddress[:maximumRemoteAddressBytes]
	}

	return IngressReceipt{
		AgentID:         current.agentID,
		SessionID:       current.sessionID,
		TraceID:         current.traceID(frame.Header.Sequence),
		RemoteAddress:   remoteAddress,
		MessageType:     frame.Header.Type,
		MessageSequence: frame.Header.Sequence,
		OperationID:     frame.Header.OperationID,
	}
}

func (current *session) traceID(sequence uint64) [16]byte {
	var input [24]byte

	copy(input[:16], current.sessionID[:])
	binary.BigEndian.PutUint64(input[16:], sequence)
	digest := sha256.Sum256(input[:])

	var traceID [16]byte

	copy(traceID[:], digest[:16])

	return traceID
}

func (current *session) sendMessageAcknowledgement(ctx context.Context, sequence uint64, result IngressResult) error {
	presence := uint64(0)
	if result.ReceiptID != [16]byte{} {
		presence |= 1 << 0
	}

	if strings.TrimSpace(result.PublicErrorCode) != "" {
		presence |= 1 << 1
	}

	acknowledgement := wire.MessageAck{
		Presence:         presence,
		OriginalSequence: sequence,
		Status:           uint64(result.Status),
		Commit:           uint64(result.Commit),
		ReceiptID:        result.ReceiptID,
		Retry:            uint64(result.Retry),
		PublicErrorCode:  result.PublicErrorCode,
	}

	body, err := acknowledgement.MarshalBinary()
	if err != nil {
		return err
	}

	return current.enqueueControl(ctx, outboundControl{
		messageType: wire.MessageMessageAck,
		flags:       wire.FlagIsResponse | wire.FlagHasCorrelation,
		correlation: sequence,
		body:        body,
	})
}

func requiresOperationID(messageType wire.MessageType) bool {
	switch messageType {
	case wire.MessageAttestation, wire.MessageCommandResult, wire.MessageCommandState, wire.MessageTransferOpen,
		wire.MessageTransferChunk, wire.MessageTransferFinalize, wire.MessageTransferAbort:
		return true
	default:
		return false
	}
}

type operationWaitGroup struct {
	waiter sync.WaitGroup
}

func (group *operationWaitGroup) start(operation func()) {
	group.waiter.Add(1)

	go func() {
		defer group.waiter.Done()
		operation()
	}()
}

func (group *operationWaitGroup) wait() {
	group.waiter.Wait()
}
