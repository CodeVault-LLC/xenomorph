package agentquic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const fallbackTransferStreamTimeout = 30 * time.Second

type transferSession struct {
	owner       *clientSession
	stream      *quic.Stream
	codec       wire.FrameCodec
	request     fileprotocol.TransferRequest
	operationID [16]byte

	mu     sync.Mutex
	closed bool
}

// PutChunk sends one capability-bound agent-authored chunk and waits for its
// durable gateway acknowledgement.
func (client *Client) PutChunk(ctx context.Context, transferID, token string, index int, data []byte) error {
	transfer, err := client.transferSession(ctx, transferID, token)
	if err != nil {
		return err
	}

	transfer.mu.Lock()
	defer transfer.mu.Unlock()

	if err := setTransferStreamDeadline(ctx, transfer.stream); err != nil {
		return err
	}

	manifestChunk, err := transfer.manifestChunk(index)
	if err != nil {
		return err
	}

	body, digest, chunkIndex, err := encodeTransferChunk(index, data, manifestChunk)
	if err != nil {
		return err
	}

	sequence := transfer.owner.nextSequence()
	if err := transfer.codec.WriteFrame(transfer.stream, wire.Frame{
		Header: wire.FrameHeader{
			Type: wire.MessageTransferChunk, SchemaRevision: 1,
			Flags:    wire.FlagAckRequired | wire.FlagHasOperationID | wire.FlagSensitive,
			Sequence: sequence, OperationID: transfer.operationID,
		}, Body: body,
	}); err != nil {
		client.transfers.clearSession(transferID, transfer)
		return fmt.Errorf("send QUIC transfer chunk: %w", err)
	}

	if err := transfer.readChunkAcknowledgement(sequence, chunkIndex, digest); err != nil {
		client.transfers.clearSession(transferID, transfer)
		return err
	}

	return nil
}

func encodeTransferChunk(
	index int,
	data []byte,
	manifestChunk fileprotocol.ChunkManifest,
) ([]byte, [sha256.Size]byte, uint64, error) {
	digest := sha256.Sum256(data)
	if int64(len(data)) != manifestChunk.Size || hex.EncodeToString(digest[:]) != manifestChunk.SHA256 {
		return nil, digest, 0, fmt.Errorf("send QUIC transfer chunk: signed manifest mismatch")
	}

	chunkIndex, err := uint64FromInt(index, "transfer chunk index")
	if err != nil {
		return nil, digest, 0, err
	}

	chunkLength, err := uint64FromInt(len(data), "transfer chunk length")
	if err != nil {
		return nil, digest, 0, err
	}

	body, err := (wire.TransferChunk{
		ChunkIndex: chunkIndex, ChunkLength: chunkLength, DigestAlgorithm: 1,
		Digest: digest, Data: data,
	}).MarshalBinary()

	return body, digest, chunkIndex, err
}

func (transfer *transferSession) readChunkAcknowledgement(
	sequence uint64,
	chunkIndex uint64,
	digest [sha256.Size]byte,
) error {
	frame, err := transfer.codec.ReadFrame(transfer.stream)
	if err != nil {
		return fmt.Errorf("read QUIC transfer chunk acknowledgement: %w", err)
	}

	if err := transfer.owner.replay.Accept(frame.Header.Sequence); err != nil {
		return err
	}

	var acknowledgement wire.TransferChunkAck
	if frame.Header.Type != wire.MessageTransferChunkAck || frame.Header.CorrelationSequence != sequence ||
		frame.Header.OperationID != transfer.operationID || acknowledgement.UnmarshalBinary(frame.Body) != nil ||
		acknowledgement.ChunkIndex != chunkIndex || acknowledgement.Digest != digest {
		return fmt.Errorf("validate QUIC transfer chunk acknowledgement: %w", wire.ErrUnexpectedMessage)
	}

	return nil
}

// GetChunk receives one gateway-authored chunk from the signed transfer stream.
func (client *Client) GetChunk(ctx context.Context, transferID, token string, index int, expectedSize int64) ([]byte, error) {
	transfer, err := client.transferSession(ctx, transferID, token)
	if err != nil {
		return nil, err
	}

	transfer.mu.Lock()
	defer transfer.mu.Unlock()

	if err := setTransferStreamDeadline(ctx, transfer.stream); err != nil {
		return nil, err
	}

	manifestChunk, err := transfer.manifestChunk(index)
	if err != nil || manifestChunk.Size != expectedSize {
		return nil, fmt.Errorf("receive QUIC transfer chunk: signed manifest mismatch")
	}

	chunkIndex, data, err := transfer.receiveChunk()
	if err != nil {
		client.transfers.clearSession(transferID, transfer)
		return nil, err
	}

	if chunkIndex != index {
		client.transfers.clearSession(transferID, transfer)
		return nil, fmt.Errorf("receive QUIC transfer chunk: noncanonical chunk order")
	}

	return data, nil
}

// Finalize submits the complete signed manifest digest to the gateway commit point.
func (client *Client) Finalize(ctx context.Context, transferID, token string) error {
	transfer, err := client.transferSession(ctx, transferID, token)
	if err != nil {
		return err
	}

	transfer.mu.Lock()
	if err := setTransferStreamDeadline(ctx, transfer.stream); err != nil {
		transfer.mu.Unlock()
		return err
	}

	body, err := encodeTransferFinalize(transfer.request)
	if err != nil {
		transfer.mu.Unlock()
		return err
	}

	if err := transfer.writeTransferFinalize(ctx, body); err != nil {
		transfer.mu.Unlock()
		return err
	}

	transfer.closed = true
	closeError := transfer.stream.Close()
	transfer.mu.Unlock()
	client.transfers.complete(transferID, transfer)

	return closeError
}

func encodeTransferFinalize(request fileprotocol.TransferRequest) ([]byte, error) {
	if request.Manifest.Direction != fileprotocol.TransferDownload {
		return nil, fmt.Errorf("finalize QUIC transfer: agent is not the byte sender")
	}

	objectDigest, err := hex.DecodeString(request.Manifest.SHA256)
	if err != nil || len(objectDigest) != sha256.Size {
		return nil, fmt.Errorf("finalize QUIC transfer: invalid signed object digest")
	}

	var digest [sha256.Size]byte

	copy(digest[:], objectDigest)

	chunkCount, err := uint64FromInt(len(request.Manifest.Chunks), "transfer chunk count")
	if err != nil {
		return nil, err
	}

	totalSize, err := uint64FromInt64(request.Manifest.Size, "transfer total size")
	if err != nil {
		return nil, err
	}

	return (wire.TransferFinalize{
		ExpectedChunkCount: chunkCount, TotalSize: totalSize, WholeObjectDigest: digest,
	}).MarshalBinary()
}

func (transfer *transferSession) writeTransferFinalize(ctx context.Context, body []byte) error {
	sequence := transfer.owner.nextSequence()
	response := make(chan eventResponse, 1)
	transfer.owner.registerPending(sequence, response)

	if err := transfer.codec.WriteFrame(transfer.stream, wire.Frame{
		Header: wire.FrameHeader{
			Type: wire.MessageTransferFinalize, SchemaRevision: 1,
			Flags:    wire.FlagAckRequired | wire.FlagHasOperationID | wire.FlagSensitive | wire.FlagEndOperation,
			Sequence: sequence, OperationID: transfer.operationID,
		}, Body: body,
	}); err != nil {
		transfer.owner.completePending(sequence, eventResponse{err: err})
		return err
	}
	select {
	case response := <-response:
		if response.err != nil {
			return response.err
		}
	case <-ctx.Done():
		transfer.owner.completePending(sequence, eventResponse{err: ctx.Err()})
		return fmt.Errorf("finalize QUIC transfer: %w", ctx.Err())
	}

	return nil
}

func (client *Client) transferSession(ctx context.Context, transferID, token string) (*transferSession, error) {
	request, err := client.transfers.contract(transferID, token)
	if err != nil {
		return nil, err
	}

	if existing := client.transfers.session(transferID); existing != nil {
		return existing, nil
	}

	owner, err := client.waitSession(ctx)
	if err != nil {
		return nil, err
	}

	created, err := openTransferSession(ctx, owner, request)
	if err != nil {
		return nil, err
	}

	client.transfers.setSession(transferID, created)

	return created, nil
}

func openTransferSession(
	ctx context.Context,
	owner *clientSession,
	request fileprotocol.TransferRequest,
) (*transferSession, error) {
	stream, codec, err := openTransferLane(ctx, owner)
	if err != nil {
		return nil, err
	}

	keepStream := false

	defer func() {
		if !keepStream {
			_ = stream.Close()
		}
	}()

	operationID, err := parseOperationID(request.Manifest.TransferID)
	if err != nil {
		return nil, err
	}

	body, err := encodeTransferOpen(request, operationID)
	if err != nil {
		return nil, err
	}

	if err := authorizeTransferLane(ctx, owner, stream, codec, operationID, body); err != nil {
		return nil, err
	}

	keepStream = true

	return &transferSession{
		owner: owner, stream: stream, codec: codec, request: request,
		operationID: operationID,
	}, nil
}

func openTransferLane(ctx context.Context, owner *clientSession) (*quic.Stream, wire.FrameCodec, error) {
	stream, err := owner.connection.OpenStreamSync(ctx)
	if err != nil {
		return nil, wire.FrameCodec{}, fmt.Errorf("open QUIC transfer stream: %w", err)
	}

	if err := setTransferStreamDeadline(ctx, stream); err != nil {
		_ = stream.Close()
		return nil, wire.FrameCodec{}, err
	}

	if err := wire.WritePreamble(stream, wire.StreamTransfer); err != nil {
		return nil, wire.FrameCodec{}, err
	}

	specification, _ := wire.SpecificationForStream(wire.StreamTransfer)
	codec, err := wire.NewFrameCodec(specification.MaximumFrameBytes)

	return stream, codec, err
}

func setTransferStreamDeadline(ctx context.Context, stream *quic.Stream) error {
	if ctx == nil || stream == nil {
		return fmt.Errorf("set QUIC transfer deadline: context and stream are required")
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("set QUIC transfer deadline: %w", err)
	}

	deadline, exists := ctx.Deadline()
	if !exists {
		deadline = time.Now().Add(fallbackTransferStreamTimeout)
	}

	if err := stream.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set QUIC transfer deadline: %w", err)
	}

	return nil
}

func encodeTransferOpen(request fileprotocol.TransferRequest, operationID [16]byte) ([]byte, error) {
	manifestDigest, err := fileprotocol.TransferManifestDigest(request.Manifest)
	if err != nil {
		return nil, err
	}

	totalSize, err := uint64FromInt64(request.Manifest.Size, "transfer total size")
	if err != nil {
		return nil, err
	}

	chunkSize, err := uint64FromInt64(request.Manifest.ChunkSize, "transfer chunk size")
	if err != nil {
		return nil, err
	}

	expiresAt, err := unixMilliseconds(request.Lease.ExpiresAt, "transfer capability expiry")
	if err != nil {
		return nil, err
	}

	direction := uint64(1)
	if request.Manifest.Direction == fileprotocol.TransferUpload {
		direction = 2
	}

	return (wire.TransferOpen{
		TransferID: operationID, SignedCapability: []byte(request.Lease.Token), Direction: direction,
		ManifestDigest: manifestDigest, ExpectedTotalSize: totalSize,
		ChunkSize: chunkSize, ExpiresAtMilliseconds: expiresAt,
	}).MarshalBinary()
}

func authorizeTransferLane(
	ctx context.Context,
	owner *clientSession,
	stream *quic.Stream,
	codec wire.FrameCodec,
	operationID [16]byte,
	body []byte,
) error {
	sequence := owner.nextSequence()
	response := make(chan eventResponse, 1)
	owner.registerPending(sequence, response)

	if err := codec.WriteFrame(stream, wire.Frame{
		Header: wire.FrameHeader{
			Type: wire.MessageTransferOpen, SchemaRevision: 1,
			Flags:    wire.FlagAckRequired | wire.FlagHasOperationID | wire.FlagSensitive,
			Sequence: sequence, OperationID: operationID,
		}, Body: body,
	}); err != nil {
		owner.completePending(sequence, eventResponse{err: err})
		return err
	}
	select {
	case acknowledgement := <-response:
		if acknowledgement.err != nil {
			return acknowledgement.err
		}
	case <-ctx.Done():
		owner.completePending(sequence, eventResponse{err: ctx.Err()})
		return fmt.Errorf("authorize QUIC transfer stream: %w", ctx.Err())
	}

	return nil
}

func (transfer *transferSession) receiveChunk() (int, []byte, error) {
	frame, chunk, chunkIndex, err := transfer.readInboundChunk()
	if err != nil {
		return 0, nil, err
	}

	if err := transfer.writeInboundChunkAcknowledgement(frame, chunk); err != nil {
		return 0, nil, err
	}

	return chunkIndex, append([]byte(nil), chunk.Data...), nil
}

func (transfer *transferSession) readInboundChunk() (wire.Frame, wire.TransferChunk, int, error) {
	frame, err := transfer.readInboundTransferFrame()
	if err != nil {
		return wire.Frame{}, wire.TransferChunk{}, 0, err
	}

	chunk, chunkIndex, err := transfer.decodeInboundTransferChunk(frame.Body)

	return frame, chunk, chunkIndex, err
}

func (transfer *transferSession) readInboundTransferFrame() (wire.Frame, error) {
	frame, err := transfer.codec.ReadFrame(transfer.stream)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return wire.Frame{}, fmt.Errorf("receive QUIC transfer chunk: gateway ended stream")
		}

		return wire.Frame{}, err
	}

	if err := transfer.owner.replay.Accept(frame.Header.Sequence); err != nil {
		return wire.Frame{}, err
	}

	if frame.Header.Type != wire.MessageTransferChunk || frame.Header.OperationID != transfer.operationID {
		return wire.Frame{}, wire.ErrUnexpectedMessage
	}

	return frame, nil
}

func (transfer *transferSession) decodeInboundTransferChunk(body []byte) (wire.TransferChunk, int, error) {
	var chunk wire.TransferChunk
	if err := chunk.UnmarshalBinary(body); err != nil {
		return wire.TransferChunk{}, 0, err
	}

	if err := wire.ValidateTransferChunk(chunk); err != nil {
		return wire.TransferChunk{}, 0, err
	}

	chunkIndex, err := intFromUint64(chunk.ChunkIndex, maximumTransferChunks, "transfer chunk index")
	if err != nil {
		return wire.TransferChunk{}, 0, err
	}

	manifestChunk, err := transfer.manifestChunk(chunkIndex)
	chunkLength, lengthErr := int64FromUint64(chunk.ChunkLength, "transfer chunk length")

	if err != nil || lengthErr != nil || chunkLength != manifestChunk.Size || hex.EncodeToString(chunk.Digest[:]) != manifestChunk.SHA256 {
		return wire.TransferChunk{}, 0, fmt.Errorf("receive QUIC transfer chunk: signed manifest mismatch")
	}

	return chunk, chunkIndex, nil
}

func (transfer *transferSession) writeInboundChunkAcknowledgement(frame wire.Frame, chunk wire.TransferChunk) error {
	acknowledgementBody, err := (wire.TransferChunkAck{ChunkIndex: chunk.ChunkIndex, Digest: chunk.Digest}).MarshalBinary()
	if err != nil {
		return err
	}

	if err := transfer.codec.WriteFrame(transfer.stream, wire.Frame{
		Header: wire.FrameHeader{
			Type: wire.MessageTransferChunkAck, SchemaRevision: 1,
			Flags:    wire.FlagIsResponse | wire.FlagHasCorrelation | wire.FlagHasOperationID,
			Sequence: transfer.owner.nextSequence(), CorrelationSequence: frame.Header.Sequence,
			OperationID: transfer.operationID,
		}, Body: acknowledgementBody,
	}); err != nil {
		return err
	}

	return nil
}

func (transfer *transferSession) manifestChunk(index int) (fileprotocol.ChunkManifest, error) {
	if index < 0 || index >= len(transfer.request.Manifest.Chunks) {
		return fileprotocol.ChunkManifest{}, fmt.Errorf("transfer chunk index is outside signed manifest")
	}

	chunk := transfer.request.Manifest.Chunks[index]
	if chunk.Index != index {
		return fileprotocol.ChunkManifest{}, fmt.Errorf("transfer chunk ordering is not canonical")
	}

	return chunk, nil
}

func (transfer *transferSession) close() error {
	transfer.mu.Lock()
	defer transfer.mu.Unlock()

	if transfer.closed {
		return nil
	}

	transfer.closed = true

	return transfer.stream.Close()
}
