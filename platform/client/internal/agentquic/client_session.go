package agentquic

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	quic "github.com/quic-go/quic-go"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const (
	resultQueueDepth    = 16
	telemetryQueueDepth = 8
	logQueueDepth       = 64
	controlQueueDepth   = 32
	replayWindowWidth   = 1024
	maximumSequenceGap  = 4096
	sessionWorkerCount  = 4
)

type trafficClass uint8

const (
	trafficResult trafficClass = iota + 1
	trafficTelemetry
	trafficLog
)

type eventRequest struct {
	messageType wire.MessageType
	flags       wire.FrameFlag
	operationID [16]byte
	body        []byte
	traffic     trafficClass
	context     context.Context
	response    chan eventResponse
}

type eventResponse struct {
	acknowledgement wire.MessageAck
	err             error
}

type outboundControl struct {
	messageType wire.MessageType
	flags       wire.FrameFlag
	correlation uint64
	body        []byte
}

type clientSession struct {
	connection    *quic.Conn
	controlStream *quic.Stream
	eventStream   *quic.SendStream
	controlCodec  wire.FrameCodec
	eventCodec    wire.FrameCodec
	serverHello   wire.ServerHello
	replay        *wire.ReplayWindow
	sequence      atomic.Uint64
	draining      atomic.Bool

	controlQueue   chan outboundControl
	resultQueue    chan eventRequest
	telemetryQueue chan eventRequest
	logQueue       chan eventRequest

	pendingMu sync.Mutex
	pending   map[uint64]chan eventResponse
	transfers *transferRegistry
}

func negotiateSession(ctx context.Context, connection *quic.Conn, hello wire.ClientHello) (*clientSession, error) {
	controlCodec, eventCodec, replay, err := newSessionCodecs()
	if err != nil {
		return nil, err
	}

	controlStream, serverHello, err := exchangeHello(ctx, connection, controlCodec, replay, hello)
	if err != nil {
		return nil, err
	}

	eventStream, err := openEventStream(ctx, connection)
	if err != nil {
		return nil, err
	}

	session := &clientSession{
		connection: connection, controlStream: controlStream, eventStream: eventStream,
		controlCodec: controlCodec, eventCodec: eventCodec, serverHello: serverHello,
		replay: replay, controlQueue: make(chan outboundControl, controlQueueDepth),
		resultQueue:    make(chan eventRequest, resultQueueDepth),
		telemetryQueue: make(chan eventRequest, telemetryQueueDepth),
		logQueue:       make(chan eventRequest, logQueueDepth), pending: make(map[uint64]chan eventResponse),
	}
	session.sequence.Store(1)

	return session, nil
}

func newSessionCodecs() (wire.FrameCodec, wire.FrameCodec, *wire.ReplayWindow, error) {
	controlSpecification, ok := wire.SpecificationForStream(wire.StreamControl)
	if !ok {
		return wire.FrameCodec{}, wire.FrameCodec{}, nil, fmt.Errorf("negotiate client session: control stream is not registered")
	}

	controlCodec, err := wire.NewFrameCodec(controlSpecification.MaximumFrameBytes)
	if err != nil {
		return wire.FrameCodec{}, wire.FrameCodec{}, nil, err
	}

	eventSpecification, ok := wire.SpecificationForStream(wire.StreamEvents)
	if !ok {
		return wire.FrameCodec{}, wire.FrameCodec{}, nil, fmt.Errorf("negotiate client session: event stream is not registered")
	}

	eventCodec, err := wire.NewFrameCodec(eventSpecification.MaximumFrameBytes)
	if err != nil {
		return wire.FrameCodec{}, wire.FrameCodec{}, nil, err
	}

	replay, err := wire.NewReplayWindow(replayWindowWidth, maximumSequenceGap)
	if err != nil {
		return wire.FrameCodec{}, wire.FrameCodec{}, nil, err
	}

	return controlCodec, eventCodec, replay, nil
}

func exchangeHello(
	ctx context.Context,
	connection *quic.Conn,
	codec wire.FrameCodec,
	replay *wire.ReplayWindow,
	offered wire.ClientHello,
) (*quic.Stream, wire.ServerHello, error) {
	controlStream, err := connection.OpenStreamSync(ctx)
	if err != nil {
		return nil, wire.ServerHello{}, fmt.Errorf("open QUIC control stream: %w", err)
	}

	if err := wire.WritePreamble(controlStream, wire.StreamControl); err != nil {
		return nil, wire.ServerHello{}, err
	}

	helloBody, err := offered.MarshalBinary()
	if err != nil {
		return nil, wire.ServerHello{}, fmt.Errorf("encode client hello: %w", err)
	}

	if err := codec.WriteFrame(controlStream, wire.Frame{
		Header: wire.FrameHeader{
			Type: wire.MessageClientHello, SchemaRevision: 1,
			Flags: wire.FlagAckRequired, Sequence: 1,
		},
		Body: helloBody,
	}); err != nil {
		return nil, wire.ServerHello{}, fmt.Errorf("write client hello: %w", err)
	}

	serverHello, err := readServerHello(controlStream, codec, replay, offered)

	return controlStream, serverHello, err
}

func readServerHello(
	stream *quic.Stream,
	codec wire.FrameCodec,
	replay *wire.ReplayWindow,
	offered wire.ClientHello,
) (wire.ServerHello, error) {
	serverFrame, err := codec.ReadFrame(stream)
	if err != nil {
		return wire.ServerHello{}, fmt.Errorf("read server hello: %w", err)
	}

	if serverFrame.Header.Type != wire.MessageServerHello {
		return wire.ServerHello{}, fmt.Errorf("read server hello: %w", ErrSecurityFailure)
	}

	if err := wire.ValidateMessageForStream(wire.StreamControl, serverFrame.Header.Type, serverFrame.Header.SchemaRevision); err != nil {
		return wire.ServerHello{}, fmt.Errorf("validate server hello frame: %w", ErrSecurityFailure)
	}

	if err := replay.Accept(serverFrame.Header.Sequence); err != nil {
		return wire.ServerHello{}, fmt.Errorf("validate server hello sequence: %w", ErrSecurityFailure)
	}

	var serverHello wire.ServerHello
	if err := serverHello.UnmarshalBinary(serverFrame.Body); err != nil {
		return wire.ServerHello{}, fmt.Errorf("decode server hello: %w: %v", ErrSecurityFailure, err)
	}

	if err := wire.ValidateServerHello(serverHello, offered); err != nil {
		return wire.ServerHello{}, fmt.Errorf("validate server hello: %w: %v", ErrSecurityFailure, err)
	}

	return serverHello, nil
}

func openEventStream(ctx context.Context, connection *quic.Conn) (*quic.SendStream, error) {
	eventStream, err := connection.OpenUniStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open QUIC event stream: %w", err)
	}

	if err := wire.WritePreamble(eventStream, wire.StreamEvents); err != nil {
		return nil, err
	}

	return eventStream, nil
}

func (session *clientSession) run(parent context.Context, commands chan<- *agent.CommandEnvelope) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	errorsChannel := make(chan error, sessionWorkerCount)

	var workers sync.WaitGroup

	start := func(operation func(context.Context) error) {
		workers.Add(1)

		go func() {
			defer workers.Done()

			if err := operation(ctx); err != nil && ctx.Err() == nil {
				select {
				case errorsChannel <- err:
				default:
				}
			}
		}()
	}
	start(session.writeControl)
	start(session.readControl)
	start(session.writeEvents)
	start(func(workerContext context.Context) error { return session.readCommands(workerContext, commands) })

	var runError error
	select {
	case <-parent.Done():
		runError = parent.Err()
	case <-session.connection.Context().Done():
		runError = context.Cause(session.connection.Context())
	case runError = <-errorsChannel:
	}
	cancel()
	session.failPending(runError)

	if closeErr := session.connection.CloseWithError(quic.ApplicationErrorCode(wire.ApplicationNoError), "client session ended"); closeErr != nil && runError == nil {
		runError = closeErr
	}

	workers.Wait()

	return runError
}

func (session *clientSession) sendEvent(ctx context.Context, request eventRequest) error {
	if session.draining.Load() {
		return ErrSessionDraining
	}

	request.context = ctx
	request.response = make(chan eventResponse, 1)

	queue, err := session.queueForTraffic(request.traffic)
	if err != nil {
		return err
	}
	select {
	case queue <- request:
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("enqueue QUIC event: %w", wire.ErrLimit)
	}
	select {
	case response := <-request.response:
		return response.err
	case <-ctx.Done():
		session.removePendingResponse(request.response)
		return fmt.Errorf("wait for QUIC application acknowledgement: %w: %w", ErrDeliveryUncertain, ctx.Err())
	}
}

func (session *clientSession) queueForTraffic(class trafficClass) (chan eventRequest, error) {
	switch class {
	case trafficResult:
		return session.resultQueue, nil
	case trafficTelemetry:
		return session.telemetryQueue, nil
	case trafficLog:
		return session.logQueue, nil
	default:
		return nil, fmt.Errorf("enqueue QUIC event: unknown traffic class %d", class)
	}
}

func (session *clientSession) writeEvents(ctx context.Context) error {
	for {
		request, ok := session.nextEvent(ctx)
		if !ok {
			return nil
		}

		if err := request.context.Err(); err != nil {
			request.response <- eventResponse{err: err}
			continue
		}

		sequence := session.nextSequence()
		if request.flags&wire.FlagAckRequired != 0 {
			session.registerPending(sequence, request.response)

			if err := request.context.Err(); err != nil {
				session.completePending(sequence, eventResponse{err: err})
				continue
			}
		}

		frame := wire.Frame{Header: wire.FrameHeader{
			Type: request.messageType, SchemaRevision: 1, Flags: request.flags,
			Sequence: sequence, OperationID: request.operationID,
		}, Body: request.body}
		if err := session.eventCodec.WriteFrame(session.eventStream, frame); err != nil {
			session.completePending(sequence, eventResponse{err: fmt.Errorf("write QUIC event: %w: %w", ErrDeliveryUncertain, err)})
			return err
		}

		if request.flags&wire.FlagAckRequired == 0 {
			request.response <- eventResponse{}
		}
	}
}

func (session *clientSession) nextEvent(ctx context.Context) (eventRequest, bool) {
	select {
	case request := <-session.resultQueue:
		return request, true
	default:
	}
	select {
	case request := <-session.resultQueue:
		return request, true
	case request := <-session.telemetryQueue:
		return request, true
	default:
	}
	select {
	case request := <-session.resultQueue:
		return request, true
	case request := <-session.telemetryQueue:
		return request, true
	case request := <-session.logQueue:
		return request, true
	case <-ctx.Done():
		return eventRequest{}, false
	}
}

func (session *clientSession) writeControl(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case outbound := <-session.controlQueue:
			if err := session.controlCodec.WriteFrame(session.controlStream, wire.Frame{
				Header: wire.FrameHeader{
					Type: outbound.messageType, SchemaRevision: 1, Flags: outbound.flags,
					Sequence: session.nextSequence(), CorrelationSequence: outbound.correlation,
				},
				Body: outbound.body,
			}); err != nil {
				return fmt.Errorf("write QUIC control frame: %w", err)
			}
		}
	}
}

func (session *clientSession) readControl(ctx context.Context) error {
	for {
		frame, err := session.controlCodec.ReadFrame(session.controlStream)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return fmt.Errorf("read QUIC control frame: %w", err)
		}

		if err := session.replay.Accept(frame.Header.Sequence); err != nil {
			return fmt.Errorf("validate QUIC control sequence: %w", err)
		}

		if err := wire.ValidateMessageForStream(wire.StreamControl, frame.Header.Type, frame.Header.SchemaRevision); err != nil {
			return err
		}

		if err := session.handleControlFrame(ctx, frame); err != nil {
			return err
		}
	}
}

func (session *clientSession) handleControlFrame(ctx context.Context, frame wire.Frame) error {
	switch frame.Header.Type {
	case wire.MessageMessageAck:
		return session.receiveAcknowledgement(frame)
	case wire.MessagePing:
		return session.receivePing(ctx, frame)
	case wire.MessageDrain:
		return session.receiveDrain(frame)
	case wire.MessageSessionReplaced:
		return receiveSessionReplacement(frame)
	case wire.MessageProtocolError:
		return receiveProtocolError(frame)
	default:
		return wire.ErrUnexpectedMessage
	}
}

func (session *clientSession) receiveDrain(frame wire.Frame) error {
	var drain wire.Drain
	if err := drain.UnmarshalBinary(frame.Body); err != nil {
		return err
	}

	session.draining.Store(true)

	return ErrSessionDraining
}

func receiveSessionReplacement(frame wire.Frame) error {
	var replacement wire.SessionReplaced
	if err := replacement.UnmarshalBinary(frame.Body); err != nil {
		return err
	}

	return fmt.Errorf("%w: gateway fenced this certificate session", ErrSessionReplaced)
}

func receiveProtocolError(frame wire.Frame) error {
	var protocolError wire.ProtocolError
	if err := protocolError.UnmarshalBinary(frame.Body); err != nil {
		return err
	}

	return fmt.Errorf("gateway protocol error %q", protocolError.PublicErrorCode)
}

func (session *clientSession) receiveAcknowledgement(frame wire.Frame) error {
	if frame.Header.Flags&(wire.FlagIsResponse|wire.FlagHasCorrelation) != wire.FlagIsResponse|wire.FlagHasCorrelation {
		return wire.ErrUnexpectedMessage
	}

	var acknowledgement wire.MessageAck
	if err := acknowledgement.UnmarshalBinary(frame.Body); err != nil {
		return err
	}

	if acknowledgement.OriginalSequence != frame.Header.CorrelationSequence {
		return wire.ErrUnexpectedMessage
	}

	response := eventResponse{acknowledgement: acknowledgement}
	status := wire.AcknowledgementStatus(acknowledgement.Status)

	if status != wire.AcknowledgementAccepted && status != wire.AcknowledgementDuplicate {
		response.err = fmt.Errorf("gateway rejected QUIC message: status=%d code=%q", status, acknowledgement.PublicErrorCode)
	}

	session.completePending(acknowledgement.OriginalSequence, response)

	return nil
}

func (session *clientSession) receivePing(ctx context.Context, frame wire.Frame) error {
	var ping wire.Ping
	if err := ping.UnmarshalBinary(frame.Body); err != nil {
		return err
	}

	body, err := wire.Pong(ping).MarshalBinary()
	if err != nil {
		return err
	}

	outbound := outboundControl{
		messageType: wire.MessagePong,
		flags:       wire.FlagIsResponse | wire.FlagHasCorrelation,
		correlation: frame.Header.Sequence, body: body,
	}
	select {
	case session.controlQueue <- outbound:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return wire.ErrLimit
	}
}

func (session *clientSession) readCommands(ctx context.Context, commands chan<- *agent.CommandEnvelope) error {
	stream, codec, err := session.acceptCommandLane(ctx)
	if err != nil {
		return err
	}

	return session.consumeCommands(ctx, stream, codec, commands)
}

func (session *clientSession) acceptCommandLane(ctx context.Context) (*quic.ReceiveStream, wire.FrameCodec, error) {
	stream, err := session.connection.AcceptUniStream(ctx)
	if err != nil {
		return nil, wire.FrameCodec{}, fmt.Errorf("accept QUIC command stream: %w", err)
	}

	preamble, err := wire.ReadPreamble(stream)
	if err != nil || preamble.Kind != wire.StreamCommands {
		return nil, wire.FrameCodec{}, fmt.Errorf("read QUIC command preamble: %w", wire.ErrUnexpectedMessage)
	}

	specification, _ := wire.SpecificationForStream(wire.StreamCommands)

	codec, err := wire.NewFrameCodec(specification.MaximumFrameBytes)
	if err != nil {
		return nil, wire.FrameCodec{}, err
	}

	return stream, codec, nil
}

func (session *clientSession) consumeCommands(
	ctx context.Context,
	stream *quic.ReceiveStream,
	codec wire.FrameCodec,
	commands chan<- *agent.CommandEnvelope,
) error {
	for {
		command, sequence, operation, err := session.readCommand(stream, codec)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return fmt.Errorf("read QUIC command frame: %w", err)
		}

		session.attachCommandPersistence(ctx, command, sequence, operation)
		select {
		case commands <- command:
		case <-ctx.Done():
			return ctx.Err()
		default:
			return fmt.Errorf("enqueue QUIC command: %w", wire.ErrLimit)
		}
	}
}

func (session *clientSession) readCommand(
	stream *quic.ReceiveStream,
	codec wire.FrameCodec,
) (*agent.CommandEnvelope, uint64, [16]byte, error) {
	frame, err := codec.ReadFrame(stream)
	if err != nil {
		return nil, 0, [16]byte{}, err
	}

	if err := session.replay.Accept(frame.Header.Sequence); err != nil {
		return nil, 0, [16]byte{}, err
	}

	if err := wire.ValidateMessageForStream(wire.StreamCommands, frame.Header.Type, frame.Header.SchemaRevision); err != nil {
		return nil, 0, [16]byte{}, err
	}

	if frame.Header.Type != wire.MessageCommand || frame.Header.Flags&wire.FlagHasOperationID == 0 {
		return nil, 0, [16]byte{}, wire.ErrUnexpectedMessage
	}

	command, err := decodeCommand(frame.Body)
	if err != nil {
		return nil, 0, [16]byte{}, err
	}

	operation, err := parseOperationID(command.CommandID)
	if err != nil || operation != frame.Header.OperationID {
		return nil, 0, [16]byte{}, fmt.Errorf("validate QUIC command operation ID: %w", wire.ErrEncoding)
	}

	return command, frame.Header.Sequence, operation, nil
}

func decodeCommand(body []byte) (*agent.CommandEnvelope, error) {
	var message wire.Command
	if err := message.UnmarshalBinary(body); err != nil {
		return nil, err
	}

	var command agent.CommandEnvelope
	if err := json.Unmarshal(message.SignedEnvelope, &command); err != nil {
		return nil, fmt.Errorf("decode signed QUIC command envelope: %w", err)
	}

	return &command, nil
}

func (session *clientSession) attachCommandPersistence(
	ctx context.Context,
	command *agent.CommandEnvelope,
	sequence uint64,
	operation [16]byte,
) {
	commandType := command.Type
	commandPayload := append([]byte(nil), command.Payload...)
	command.AcknowledgePersistence = func() error {
		if session.transfers != nil {
			if err := session.transfers.applySignedCommand(commandType, commandPayload); err != nil {
				return err
			}
		}

		if err := session.commitCommandAcceptance(ctx, operation); err != nil {
			return err
		}

		return session.acknowledgeCommandPersistence(ctx, sequence)
	}
}

func (session *clientSession) commitCommandAcceptance(ctx context.Context, operationID [16]byte) error {
	body, err := (wire.CommandState{State: 1}).MarshalBinary()
	if err != nil {
		return err
	}

	err = session.sendEvent(ctx, eventRequest{
		messageType: wire.MessageCommandState,
		flags:       wire.FlagAckRequired | wire.FlagHasOperationID | wire.FlagSensitive,
		operationID: operationID, body: body, traffic: trafficResult,
	})

	return err
}

func (session *clientSession) acknowledgeCommandPersistence(ctx context.Context, sequence uint64) error {
	body, err := (wire.MessageAck{
		OriginalSequence: sequence,
		Status:           uint64(wire.AcknowledgementAccepted),
		Commit:           uint64(wire.CommitPersisted),
		Retry:            uint64(wire.RetryNever),
	}).MarshalBinary()
	if err != nil {
		return err
	}

	outbound := outboundControl{
		messageType: wire.MessageMessageAck,
		flags:       wire.FlagIsResponse | wire.FlagHasCorrelation,
		correlation: sequence, body: body,
	}
	select {
	case session.controlQueue <- outbound:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return wire.ErrLimit
	}
}

func (session *clientSession) nextSequence() uint64 {
	return session.sequence.Add(1)
}

func (session *clientSession) registerPending(sequence uint64, response chan eventResponse) {
	session.pendingMu.Lock()
	session.pending[sequence] = response
	session.pendingMu.Unlock()
}

func (session *clientSession) completePending(sequence uint64, response eventResponse) {
	session.pendingMu.Lock()
	channel := session.pending[sequence]
	delete(session.pending, sequence)
	session.pendingMu.Unlock()

	if channel != nil {
		channel <- response
	}
}

func (session *clientSession) removePendingResponse(response chan eventResponse) {
	session.pendingMu.Lock()
	defer session.pendingMu.Unlock()

	for sequence, pendingResponse := range session.pending {
		if pendingResponse == response {
			delete(session.pending, sequence)
			return
		}
	}
}

func (session *clientSession) failPending(cause error) {
	if cause == nil {
		cause = ErrDeliveryUncertain
	}

	session.pendingMu.Lock()
	pending := session.pending
	session.pending = make(map[uint64]chan eventResponse)
	session.pendingMu.Unlock()

	for _, channel := range pending {
		channel <- eventResponse{err: fmt.Errorf("%w: %v", ErrDeliveryUncertain, cause)}
	}
}
