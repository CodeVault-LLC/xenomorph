package agentquic

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	quic "github.com/quic-go/quic-go"

	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const (
	controlQueueDepth  = 64
	replayWindowWidth  = 1024
	maximumSequenceGap = 4096
	sessionWorkerCount = 5
)

type session struct {
	listener       *Listener
	connection     *quic.Conn
	agentID        string
	sessionID      [16]byte
	generation     uint64
	fenced         atomic.Bool
	sequence       atomic.Uint64
	replay         *wire.ReplayWindow
	controlStream  *quic.Stream
	controlCodec   wire.FrameCodec
	controlQueue   chan outboundControl
	replacement    chan [16]byte
	eventClaimed   atomic.Bool
	mediaWorkers   chan struct{}
	transferWorker chan struct{}
	hello          wire.ClientHello
}

type outboundControl struct {
	messageType wire.MessageType
	flags       wire.FrameFlag
	correlation uint64
	body        []byte
	done        chan error
}

func newSession(listener *Listener, connection *quic.Conn, agentID string, sessionID [16]byte) (*session, error) {
	controlSpec, ok := wire.SpecificationForStream(wire.StreamControl)
	if !ok {
		return nil, fmt.Errorf("create QUIC session: missing control stream specification")
	}

	controlCodec, err := wire.NewFrameCodec(controlSpec.MaximumFrameBytes)
	if err != nil {
		return nil, err
	}

	replay, err := wire.NewReplayWindow(replayWindowWidth, maximumSequenceGap)
	if err != nil {
		return nil, err
	}

	return &session{
		listener:       listener,
		connection:     connection,
		agentID:        agentID,
		sessionID:      sessionID,
		replay:         replay,
		controlCodec:   controlCodec,
		controlQueue:   make(chan outboundControl, controlQueueDepth),
		replacement:    make(chan [16]byte, 1),
		mediaWorkers:   make(chan struct{}, 1),
		transferWorker: make(chan struct{}, listener.config.ConcurrentTransferStreams),
	}, nil
}

func (current *session) negotiate(ctx context.Context) error {
	negotiationContext, cancel := context.WithTimeout(ctx, current.listener.config.ControlStreamTimeout)
	defer cancel()

	stream, err := current.acceptControlStream(negotiationContext)
	if err != nil {
		return err
	}

	if err := current.readClientHello(stream); err != nil {
		return err
	}

	if err := current.writeServerHello(stream); err != nil {
		return err
	}

	if err := stream.SetDeadline(time.Time{}); err != nil {
		return fmt.Errorf("clear control negotiation deadline: %w", err)
	}

	current.controlStream = stream

	return nil
}

func (current *session) acceptControlStream(ctx context.Context) (*quic.Stream, error) {
	stream, err := current.connection.AcceptStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("accept control stream: %w", err)
	}

	streamID, err := streamIDValue(int64(stream.StreamID()))
	if err != nil || streamID&0x3 != 0 {
		return nil, fmt.Errorf("accept control stream: %w: wrong initiator or direction", wire.ErrUnexpectedMessage)
	}

	if err := stream.SetDeadline(time.Now().Add(current.listener.config.ControlStreamTimeout)); err != nil {
		return nil, fmt.Errorf("set control negotiation deadline: %w", err)
	}

	preamble, err := wire.ReadPreamble(stream)
	if err != nil || preamble.Kind != wire.StreamControl {
		return nil, fmt.Errorf("read control preamble: %w", wire.ErrUnexpectedMessage)
	}

	return stream, nil
}

func (current *session) readClientHello(stream *quic.Stream) error {
	frame, err := current.controlCodec.ReadFrame(stream)
	if err != nil {
		return err
	}

	if err := wire.ValidateMessageForStream(wire.StreamControl, frame.Header.Type, frame.Header.SchemaRevision); err != nil || frame.Header.Type != wire.MessageClientHello {
		return fmt.Errorf("read client hello: %w", wire.ErrUnexpectedMessage)
	}

	if err := current.replay.Accept(frame.Header.Sequence); err != nil {
		return err
	}

	if err := current.hello.UnmarshalBinary(frame.Body); err != nil {
		return fmt.Errorf("decode client hello: %w", err)
	}

	if err := wire.ValidateClientHello(current.hello); err != nil {
		return err
	}

	return nil
}

func (current *session) writeServerHello(stream *quic.Stream) error {
	serverHello := current.serverHello()

	body, err := serverHello.MarshalBinary()
	if err != nil {
		return fmt.Errorf("encode server hello: %w", err)
	}

	if err := current.controlCodec.WriteFrame(stream, wire.Frame{
		Header: wire.FrameHeader{Type: wire.MessageServerHello, SchemaRevision: 1, Sequence: current.nextSequence()},
		Body:   body,
	}); err != nil {
		return fmt.Errorf("write server hello: %w", err)
	}

	return nil
}

func (current *session) serverHello() wire.ServerHello {
	heartbeatMilliseconds, _ := durationMilliseconds(current.listener.config.HeartbeatInterval)
	idleMilliseconds, _ := durationMilliseconds(current.listener.config.MaximumIdleTimeout)

	return wire.ServerHello{
		SelectedMinor:                 uint64(wire.ProtocolMinor),
		NegotiatedFeatures:            0,
		SessionID:                     current.sessionID,
		HeartbeatIntervalMilliseconds: heartbeatMilliseconds,
		MaximumIdleMilliseconds:       idleMilliseconds,
		EventFrameMaximum:             uint64(current.listener.config.EventFrameMaximum),
		ConcurrentTransferStreams:     uint64(current.listener.config.ConcurrentTransferStreams),
		CommandVerificationKeyID:      current.listener.commandKeyID,
	}
}

func (current *session) run(parent context.Context) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	defer cancel()

	commandStream, err := current.openCommandStream(ctx)
	if err != nil {
		closeConnection(current.connection, wire.ApplicationInternal, "command lane unavailable")
		return
	}

	errors := make(chan error, sessionWorkerCount)

	var workers sync.WaitGroup

	start := func(operation func(context.Context) error) {
		workers.Add(1)

		go func() {
			defer workers.Done()

			if err := operation(ctx); err != nil && ctx.Err() == nil {
				select {
				case errors <- err:
				default:
				}
			}
		}()
	}
	start(current.writeControl)
	start(current.readControl)
	start(current.acceptUnidirectionalStreams)
	start(current.acceptBidirectionalStreams)
	start(func(workerContext context.Context) error { return current.writeCommands(workerContext, commandStream) })

	closeCode := wire.ApplicationNoError
	closeDescription := "drain complete"
	select {
	case <-parent.Done():
		current.sendDrain()
	case replacementID := <-current.replacement:
		current.sendReplacement(replacementID)

		closeCode = wire.ApplicationSessionReplaced
		closeDescription = "session replaced"
	case <-current.connection.Context().Done():
	case err := <-errors:
		closeCode = errorCode(err)
		closeDescription = "session protocol failure"
	}
	cancel()
	closeConnection(current.connection, closeCode, closeDescription)
	workers.Wait()
}

func (current *session) replace(replacementID [16]byte) {
	select {
	case current.replacement <- replacementID:
	default:
		closeConnection(current.connection, wire.ApplicationSessionReplaced, "session replaced")
	}
}

func (current *session) nextSequence() uint64 {
	return current.sequence.Add(1)
}

func (current *session) openCommandStream(ctx context.Context) (*quic.SendStream, error) {
	stream, err := current.connection.OpenUniStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open command stream: %w", err)
	}

	if err := wire.WritePreamble(stream, wire.StreamCommands); err != nil {
		return nil, err
	}

	return stream, nil
}

func (current *session) writeControl(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case outbound := <-current.controlQueue:
			header := wire.FrameHeader{
				Type:                outbound.messageType,
				SchemaRevision:      1,
				Flags:               outbound.flags,
				Sequence:            current.nextSequence(),
				CorrelationSequence: outbound.correlation,
			}

			err := current.controlCodec.WriteFrame(current.controlStream, wire.Frame{Header: header, Body: outbound.body})
			if outbound.done != nil {
				outbound.done <- err
			}

			if err != nil {
				return fmt.Errorf("write control frame: %w", err)
			}
		}
	}
}

func (current *session) readControl(ctx context.Context) error {
	for {
		frame, err := current.controlCodec.ReadFrame(current.controlStream)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return fmt.Errorf("read control frame: %w", err)
		}

		if err := current.replay.Accept(frame.Header.Sequence); err != nil {
			return err
		}

		if err := wire.ValidateMessageForStream(wire.StreamControl, frame.Header.Type, frame.Header.SchemaRevision); err != nil {
			return err
		}

		if err := current.handleControlFrame(ctx, frame); err != nil {
			return err
		}
	}
}

func (current *session) handleControlFrame(ctx context.Context, frame wire.Frame) error {
	switch frame.Header.Type {
	case wire.MessagePing:
		return current.respondToPing(ctx, frame)
	case wire.MessagePong:
		var pong wire.Pong
		return decodeCorrelatedControl(frame, &pong, 0)
	case wire.MessageMessageAck:
		var acknowledgement wire.MessageAck
		return decodeCorrelatedControl(frame, &acknowledgement, frame.Header.CorrelationSequence)
	default:
		return wire.ErrUnexpectedMessage
	}
}

type binaryUnmarshaler interface {
	UnmarshalBinary([]byte) error
}

func decodeCorrelatedControl(frame wire.Frame, body binaryUnmarshaler, originalSequence uint64) error {
	if err := body.UnmarshalBinary(frame.Body); err != nil {
		return err
	}

	correlatedFlags := wire.FlagIsResponse | wire.FlagHasCorrelation
	if frame.Header.Flags&correlatedFlags != correlatedFlags {
		return wire.ErrUnexpectedMessage
	}

	if acknowledgement, ok := body.(*wire.MessageAck); ok && acknowledgement.OriginalSequence != originalSequence {
		return wire.ErrUnexpectedMessage
	}

	return nil
}

func (current *session) respondToPing(ctx context.Context, frame wire.Frame) error {
	var ping wire.Ping
	if err := ping.UnmarshalBinary(frame.Body); err != nil {
		return err
	}

	body, err := wire.Pong(ping).MarshalBinary()
	if err != nil {
		return err
	}

	return current.enqueueControl(ctx, outboundControl{
		messageType: wire.MessagePong,
		flags:       wire.FlagIsResponse | wire.FlagHasCorrelation,
		correlation: frame.Header.Sequence,
		body:        body,
	})
}

func (current *session) writeCommands(ctx context.Context, stream *quic.SendStream) error {
	specification, _ := wire.SpecificationForStream(wire.StreamCommands)

	codec, err := wire.NewFrameCodec(specification.MaximumFrameBytes)
	if err != nil {
		return err
	}

	for {
		if current.fenced.Load() {
			return nil
		}

		command, err := current.listener.commands.WaitDispatch(ctx, current.agentID)
		if err != nil {
			return err
		}

		if command == nil {
			return nil
		}

		if current.fenced.Load() {
			return nil
		}

		frame, err := current.commandFrame(command)
		if err != nil {
			return current.failDispatchedCommand(command.CommandID, err)
		}

		if err := codec.WriteFrame(stream, frame); err != nil {
			return current.failDispatchedCommand(command.CommandID, fmt.Errorf("write command frame: %w", err))
		}
	}
}

func (current *session) commandFrame(command *commandauth.Envelope) (wire.Frame, error) {
	body, err := json.Marshal(command)
	if err != nil {
		return wire.Frame{}, fmt.Errorf("encode signed command envelope: %w", err)
	}

	messageBody, err := (wire.Command{SignedEnvelope: body}).MarshalBinary()
	if err != nil {
		return wire.Frame{}, err
	}

	operationID, err := uuid.Parse(command.CommandID)
	if err != nil {
		return wire.Frame{}, fmt.Errorf("parse gateway command ID: %w", err)
	}

	return wire.Frame{
		Header: wire.FrameHeader{
			Type: wire.MessageCommand, SchemaRevision: 1,
			Flags:    wire.FlagAckRequired | wire.FlagHasOperationID | wire.FlagSensitive,
			Sequence: current.nextSequence(), OperationID: [16]byte(operationID),
		},
		Body: messageBody,
	}, nil
}

func (current *session) failDispatchedCommand(commandID string, cause error) error {
	if err := current.listener.commands.MarkOutcomeUnknown(current.agentID, commandID); err != nil {
		return fmt.Errorf("%v; persist ambiguous command outcome: %w", cause, err)
	}

	return cause
}

func (current *session) enqueueControl(ctx context.Context, outbound outboundControl) error {
	select {
	case current.controlQueue <- outbound:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("enqueue control frame: %w", wire.ErrLimit)
	}
}

func (current *session) sendDrain() {
	deadlineMilliseconds, err := durationMilliseconds(current.listener.config.DrainTimeout)
	if err != nil {
		return
	}

	body, err := (wire.Drain{DeadlineMilliseconds: deadlineMilliseconds, Reason: 1}).MarshalBinary()
	if err != nil {
		return
	}

	current.sendTerminalControl(outboundControl{messageType: wire.MessageDrain, flags: wire.FlagAckRequired, body: body})
}

func (current *session) sendReplacement(replacementID [16]byte) {
	body, err := (wire.SessionReplaced{ReplacementSessionID: replacementID}).MarshalBinary()
	if err != nil {
		return
	}

	current.sendTerminalControl(outboundControl{messageType: wire.MessageSessionReplaced, body: body})
}

func (current *session) sendTerminalControl(outbound outboundControl) {
	done := make(chan error, 1)
	outbound.done = done

	timer := time.NewTimer(current.listener.config.ControlStreamTimeout)
	defer timer.Stop()
	select {
	case current.controlQueue <- outbound:
	case <-timer.C:
		return
	}
	select {
	case <-done:
	case <-timer.C:
	}
}
