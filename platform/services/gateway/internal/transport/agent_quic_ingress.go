package transport

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/agentquic"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/fileworkspace"
	operationjournal "github.com/codevault-llc/xenomorph/platform/services/gateway/internal/operation"
	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const (
	agentIngressCommitTimeout = 10 * time.Second
	maximumTransferChunkIndex = 1_048_576
	maximumLogDetailBytes     = 512
	maximumClientIPBytes      = 64
	maximumReportedCPUCores   = 4096
	maximumReportedCPUThreads = 8192
	partsPerMillion           = 1_000_000
	uuidVersionMask           = 0x0f
	uuidVersionFive           = 0x50
	uuidVariantMask           = 0x3f
	uuidRFC4122Variant        = 0x80
)

// CommitAgentMessage validates and commits one canonical agent-authored XBP body.
// The caller holds the current authenticated-session lease for the full call.
func (s *Server) CommitAgentMessage(ctx context.Context, receipt agentquic.IngressReceipt, message agentquic.IngressMessage) (agentquic.IngressResult, error) {
	if ctx == nil {
		return failedIngressResult("invalid_context"), fmt.Errorf("commit agent message: context is nil")
	}
	if s == nil || s.broker == nil || strings.TrimSpace(receipt.AgentID) == "" ||
		receipt.SessionID == [16]byte{} || receipt.TraceID == [16]byte{} || receipt.MessageType != message.Type {
		return failedIngressResult("invalid_receipt"), fmt.Errorf("commit agent message: invalid authenticated receipt")
	}
	commitContext, cancel := context.WithTimeout(ctx, agentIngressCommitTimeout)
	defer cancel()
	return s.commitQUICMessage(commitContext, receipt, message)
}

func (s *Server) commitQUICMessage(ctx context.Context, receipt agentquic.IngressReceipt, message agentquic.IngressMessage) (agentquic.IngressResult, error) {
	switch message.Type {
	case wire.MessageHeartbeat, wire.MessageAttestation, wire.MessageLogEntry,
		wire.MessageCommandResult, wire.MessageCommandState:
		return s.commitQUICEvent(ctx, receipt, message)
	case wire.MessageTransferOpen, wire.MessageTransferChunk, wire.MessageTransferFinalize, wire.MessageTransferAbort:
		return s.commitQUICTransfer(receipt, message)
	case wire.MessageMediaOpen, wire.MessageMediaFrame, wire.MessageMediaClose:
		return s.commitQUICMedia(receipt, message)
	default:
		return failedIngressResult("unsupported_family"), fmt.Errorf("commit agent message: message family is not enabled")
	}
}

func (s *Server) commitQUICEvent(ctx context.Context, receipt agentquic.IngressReceipt, message agentquic.IngressMessage) (agentquic.IngressResult, error) {
	switch message.Type {
	case wire.MessageHeartbeat:
		return s.commitQUICHeartbeat(ctx, receipt, message.Heartbeat)
	case wire.MessageAttestation:
		return s.commitQUICAttestation(ctx, receipt, message.Attestation)
	case wire.MessageLogEntry:
		return s.commitQUICLogEntry(ctx, receipt, message.LogEntry)
	case wire.MessageCommandResult:
		return s.commitQUICCommandResult(ctx, receipt, message.CommandResult)
	case wire.MessageCommandState:
		return s.commitQUICCommandState(receipt, message.CommandState)
	default:
		return failedIngressResult("unsupported_event_family"), wire.ErrUnexpectedMessage
	}
}

func (s *Server) commitQUICTransfer(receipt agentquic.IngressReceipt, message agentquic.IngressMessage) (agentquic.IngressResult, error) {
	switch message.Type {
	case wire.MessageTransferOpen:
		return s.commitQUICTransferOpen(receipt, message.TransferOpen)
	case wire.MessageTransferChunk:
		return s.commitQUICTransferChunk(receipt, message.TransferChunk)
	case wire.MessageTransferFinalize:
		return s.commitQUICTransferFinalize(receipt, message.TransferFinal)
	case wire.MessageTransferAbort:
		return s.commitQUICTransferAbort(receipt, message.TransferAbort)
	default:
		return failedIngressResult("unsupported_transfer_family"), wire.ErrUnexpectedMessage
	}
}

func (s *Server) commitQUICMedia(receipt agentquic.IngressReceipt, message agentquic.IngressMessage) (agentquic.IngressResult, error) {
	switch message.Type {
	case wire.MessageMediaOpen:
		return s.commitQUICMediaOpen(receipt, message.MediaOpen)
	case wire.MessageMediaFrame:
		return s.commitQUICMediaFrame(receipt, message.MediaFrame)
	case wire.MessageMediaClose:
		return acceptedIngressResult(wire.CommitDecoded, [16]byte{}), nil
	default:
		return failedIngressResult("unsupported_media_family"), wire.ErrUnexpectedMessage
	}
}

func (s *Server) commitQUICCommandState(receipt agentquic.IngressReceipt, state *wire.CommandState) (agentquic.IngressResult, error) {
	if state == nil || state.State != 1 || receipt.OperationID == [16]byte{} || s.commandQueue == nil {
		return failedIngressResult("invalid_command_state"), fmt.Errorf("commit command state: invalid accepted state")
	}
	commandID := uuid.UUID(receipt.OperationID).String()
	if _, exists := s.commandQueue.Command(receipt.AgentID, commandID); !exists {
		return failedIngressResult("command_scope_mismatch"), fmt.Errorf("commit command state: command audience mismatch")
	}
	if err := s.commandQueue.MarkAccepted(receipt.AgentID, commandID); err != nil {
		return failedIngressResult("command_state_conflict"), err
	}
	return acceptedIngressResult(wire.CommitPersisted, [16]byte{}), nil
}

func (s *Server) commitQUICTransferOpen(receipt agentquic.IngressReceipt, open *wire.TransferOpen) (agentquic.IngressResult, error) {
	if open == nil || s.fileWorkspace == nil || s.quicTransfers == nil {
		return failedIngressResult("transfer_unavailable"), fmt.Errorf("commit transfer open: workspace is unavailable")
	}
	if _, err := s.quicTransfers.authorize(s.fileWorkspace, receipt, *open); err != nil {
		return failedIngressResult("transfer_not_authorized"), err
	}
	return acceptedIngressResult(wire.CommitValidated, [16]byte{}), nil
}

func (s *Server) commitQUICTransferChunk(receipt agentquic.IngressReceipt, chunk *wire.TransferChunk) (agentquic.IngressResult, error) {
	if chunk == nil || s.fileWorkspace == nil || s.quicTransfers == nil {
		return failedIngressResult("transfer_unavailable"), fmt.Errorf("commit transfer chunk: workspace is unavailable")
	}
	capability, err := s.quicTransfers.capability(receipt.AgentID, receipt.OperationID)
	if err != nil || capability.direction != 1 {
		return failedIngressResult("transfer_not_authorized"), fmt.Errorf("commit transfer chunk: capability direction mismatch")
	}
	chunkIndex, err := boundedIntFromUint64(chunk.ChunkIndex, maximumTransferChunkIndex, "transfer chunk index")
	if err != nil {
		return failedIngressResult("transfer_chunk_rejected"), err
	}
	if _, err := s.fileWorkspace.PutAgentTransferChunk(
		receipt.AgentID, capability.transferID, capability.token, chunkIndex, chunk.Data,
	); err != nil {
		return failedIngressResult("transfer_chunk_rejected"), err
	}
	return acceptedIngressResult(wire.CommitPersisted, [16]byte{}), nil
}

func (s *Server) commitQUICTransferFinalize(receipt agentquic.IngressReceipt, final *wire.TransferFinalize) (agentquic.IngressResult, error) {
	if final == nil || s.fileWorkspace == nil || s.quicTransfers == nil {
		return failedIngressResult("transfer_unavailable"), fmt.Errorf("commit transfer finalize: workspace is unavailable")
	}
	capability, err := s.quicTransfers.capability(receipt.AgentID, receipt.OperationID)
	if err != nil || capability.direction != 1 {
		return failedIngressResult("transfer_not_authorized"), fmt.Errorf("commit transfer finalize: capability direction mismatch")
	}
	transfer, exists := s.fileWorkspace.Transfer(receipt.AgentID, capability.transferID)
	if !exists {
		return failedIngressResult("transfer_not_found"), fmt.Errorf("commit transfer finalize: transfer not found")
	}
	if err := validateTransferFinalize(*final, transfer.Manifest); err != nil {
		return failedIngressResult("transfer_manifest_mismatch"), fmt.Errorf("commit transfer finalize: signed manifest mismatch")
	}
	if _, err := s.fileWorkspace.FinalizeAgentTransfer(receipt.AgentID, capability.transferID, capability.token); err != nil {
		return failedIngressResult("transfer_finalize_failed"), err
	}
	s.quicTransfers.complete(receipt.AgentID, receipt.OperationID)
	return acceptedIngressResult(wire.CommitPersisted, [16]byte{}), nil
}

func validateTransferFinalize(final wire.TransferFinalize, manifest fileprotocol.TransferManifest) error {
	objectDigest, err := manifestObjectDigest(manifest)
	if err != nil {
		return err
	}
	chunkCount, err := uint64FromNonnegativeInt(len(manifest.Chunks), "transfer chunk count")
	if err != nil {
		return err
	}
	totalSize, err := uint64FromNonnegativeInt64(manifest.Size, "transfer total size")
	if err != nil {
		return err
	}
	if final.ExpectedChunkCount != chunkCount || final.TotalSize != totalSize || final.WholeObjectDigest != objectDigest {
		return fmt.Errorf("validate transfer finalize: manifest mismatch")
	}
	return nil
}

func (s *Server) commitQUICTransferAbort(receipt agentquic.IngressReceipt, abort *wire.TransferAbort) (agentquic.IngressResult, error) {
	if abort == nil || s.quicTransfers == nil {
		return failedIngressResult("transfer_unavailable"), fmt.Errorf("commit transfer abort: transfer registry is unavailable")
	}
	if _, err := s.quicTransfers.capability(receipt.AgentID, receipt.OperationID); err != nil {
		return failedIngressResult("transfer_not_authorized"), err
	}
	s.quicTransfers.complete(receipt.AgentID, receipt.OperationID)
	return acceptedIngressResult(wire.CommitPersisted, [16]byte{}), nil
}

// StreamTransferChunks emits gateway-staged bytes through the authenticated
// transfer stream after CommitAgentMessage authorized its signed capability.
func (s *Server) StreamTransferChunks(
	ctx context.Context,
	receipt agentquic.IngressReceipt,
	open wire.TransferOpen,
	emit func(wire.TransferChunk) error,
) error {
	if err := s.validateTransferChunkStream(ctx, emit); err != nil {
		return err
	}
	capability, transfer, err := s.gatewayTransferSource(receipt.AgentID, open.TransferID)
	if err != nil {
		return err
	}
	if err := s.emitGatewayTransferChunks(ctx, receipt.AgentID, capability, transfer.Manifest.Chunks, emit); err != nil {
		return err
	}
	s.quicTransfers.complete(receipt.AgentID, open.TransferID)
	return nil
}

func (s *Server) validateTransferChunkStream(ctx context.Context, emit func(wire.TransferChunk) error) error {
	if ctx == nil || emit == nil || s.fileWorkspace == nil || s.quicTransfers == nil {
		return fmt.Errorf("stream transfer chunks: workspace is unavailable")
	}
	return nil
}

func (s *Server) gatewayTransferSource(agentID string, transferID [16]byte) (quicTransferCapability, fileworkspace.Transfer, error) {
	capability, err := s.quicTransfers.capability(agentID, transferID)
	if err != nil || capability.direction != 2 {
		return quicTransferCapability{}, fileworkspace.Transfer{}, fmt.Errorf("stream transfer chunks: capability direction mismatch")
	}
	transfer, exists := s.fileWorkspace.Transfer(agentID, capability.transferID)
	if !exists {
		return quicTransferCapability{}, fileworkspace.Transfer{}, fmt.Errorf("stream transfer chunks: transfer not found")
	}
	return capability, transfer, nil
}

func (s *Server) emitGatewayTransferChunks(
	ctx context.Context,
	agentID string,
	capability quicTransferCapability,
	chunks []fileprotocol.ChunkManifest,
	emit func(wire.TransferChunk) error,
) error {
	for _, manifestChunk := range chunks {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := s.gatewayTransferChunk(agentID, capability, manifestChunk.Index)
		if err != nil {
			return err
		}
		if err := emit(chunk); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) gatewayTransferChunk(agentID string, capability quicTransferCapability, index int) (wire.TransferChunk, error) {
	data, err := s.fileWorkspace.ReadAgentTransferChunk(agentID, capability.transferID, capability.token, index)
	if err != nil {
		return wire.TransferChunk{}, err
	}
	chunkIndex, err := uint64FromNonnegativeInt(index, "transfer chunk index")
	if err != nil {
		return wire.TransferChunk{}, err
	}
	chunkLength, err := uint64FromNonnegativeInt(len(data), "transfer chunk length")
	if err != nil {
		return wire.TransferChunk{}, err
	}
	return wire.TransferChunk{ChunkIndex: chunkIndex, ChunkLength: chunkLength,
		DigestAlgorithm: 1, Digest: sha256.Sum256(data), Data: data}, nil
}

func (s *Server) commitQUICHeartbeat(ctx context.Context, receipt agentquic.IngressReceipt, heartbeat *wire.Heartbeat) (agentquic.IngressResult, error) {
	if heartbeat == nil {
		return failedIngressResult("invalid_heartbeat"), fmt.Errorf("commit heartbeat: body is nil")
	}
	payload := heartbeatToProtobuf(*heartbeat)
	envelope, eventID := newIngressEnvelope(receipt)
	envelope.Payload = &pb.EventEnvelope_Heartbeat{Heartbeat: payload}
	if err := s.broker.PublishContext(ctx, "sys.in.default."+receipt.AgentID+".heartbeat", envelope); err != nil {
		return failedIngressResult("broker_unavailable"), fmt.Errorf("publish QUIC heartbeat: %w", err)
	}
	s.markSeen(receipt.AgentID)
	return acceptedIngressResult(wire.CommitPublished, eventID), nil
}

func (s *Server) commitQUICAttestation(ctx context.Context, receipt agentquic.IngressReceipt, attestation *wire.Attestation) (agentquic.IngressResult, error) {
	if attestation == nil || receipt.OperationID == [16]byte{} || s.operationJournal == nil {
		return failedIngressResult("invalid_attestation"), fmt.Errorf("commit attestation: body and operation ID are required")
	}
	canonical, err := attestation.MarshalBinary()
	if err != nil {
		return failedIngressResult("invalid_attestation"), err
	}
	disposition, err := s.operationJournal.Begin(receipt.AgentID, uint16(wire.MessageAttestation), receipt.OperationID, canonical)
	if err != nil {
		return failedIngressResult("attestation_reconciliation_required"), err
	}
	if disposition == operationjournal.Duplicate {
		_, eventID := newDeterministicIngressEnvelope(receipt, "attestation")
		result := acceptedIngressResult(wire.CommitPublished, eventID)
		result.Status = wire.AcknowledgementDuplicate
		return result, nil
	}
	browsers := make([]attestationBrowser, 0, len(attestation.Browsers))
	for _, browser := range attestation.Browsers {
		browsers = append(browsers, attestationBrowser{Name: browser.Name, BinaryPath: browser.BinaryPath, ProfileDir: browser.ProfileDirectory})
	}
	report := normalizeAttestationRequest(receipt.AgentID, attestationRequest{
		Hostname: attestation.Hostname, OSVersion: attestation.OSVersion,
		RequiresAttestation: attestation.RequiresAttestation, Browsers: browsers,
		InstalledApplications: attestation.InstalledApplications,
	})
	envelope, eventID := newDeterministicIngressEnvelope(receipt, "attestation")
	envelope.Payload = &pb.EventEnvelope_LogEntry{LogEntry: &pb.LogEntry{
		Level: "INFO", Component: "gateway.ingest.attestation",
		Message: fmt.Sprintf("endpoint attestation accepted hostname=%s browsers=%d apps=%d requires_attestation=%t", report.Hostname, len(report.Browsers), len(report.InstalledApplications), report.RequiresAttestation),
	}}
	if err := s.broker.PublishContext(ctx, "sys.in.default."+receipt.AgentID+".attestation", envelope); err != nil {
		_ = s.operationJournal.Release(receipt.AgentID, uint16(wire.MessageAttestation), receipt.OperationID)
		return failedIngressResult("broker_unavailable"), fmt.Errorf("publish QUIC attestation: %w", err)
	}
	if err := s.operationJournal.Commit(receipt.AgentID, uint16(wire.MessageAttestation), receipt.OperationID); err != nil {
		return failedIngressResult("operation_journal_unavailable"), err
	}
	s.storeLogEnvelope(envelope)
	return acceptedIngressResult(wire.CommitPublished, eventID), nil
}

func (s *Server) commitQUICLogEntry(ctx context.Context, receipt agentquic.IngressReceipt, entry *wire.LogEntry) (agentquic.IngressResult, error) {
	if entry == nil {
		return failedIngressResult("invalid_log"), fmt.Errorf("commit log entry: body is nil")
	}
	level, component, eventName, ok := logRegistryValues(*entry)
	if !ok {
		return failedIngressResult("invalid_log_registry"), fmt.Errorf("commit log entry: unassigned registry value")
	}
	message := "event=" + eventName
	if detail := strings.TrimSpace(entry.Detail); detail != "" {
		message += " detail=" + clampText(detail, maximumLogDetailBytes)
	}
	envelope, eventID := newIngressEnvelope(receipt)
	envelope.Payload = &pb.EventEnvelope_LogEntry{LogEntry: &pb.LogEntry{Level: level, Component: component, Message: message}}
	if err := s.broker.PublishContext(ctx, "sys.in.default."+receipt.AgentID+".logs", envelope); err != nil {
		return failedIngressResult("broker_unavailable"), fmt.Errorf("publish QUIC log entry: %w", err)
	}
	s.storeLogEnvelope(envelope)
	return acceptedIngressResult(wire.CommitPublished, eventID), nil
}

func (s *Server) commitQUICCommandResult(ctx context.Context, receipt agentquic.IngressReceipt, result *wire.CommandResult) (agentquic.IngressResult, error) {
	if result == nil || receipt.OperationID == [16]byte{} || s.commandQueue == nil {
		return failedIngressResult("invalid_command_result"), fmt.Errorf("commit command result: body, operation ID, and journal are required")
	}
	commandID := uuid.UUID(receipt.OperationID).String()
	envelope, exists := s.commandQueue.Command(receipt.AgentID, commandID)
	if !exists || envelope.Type != result.CommandType {
		return failedIngressResult("command_scope_mismatch"), fmt.Errorf("commit command result: command audience or type mismatch")
	}
	request := commandResultFromWire(commandID, *result)
	canonicalResult, err := canonicalizeCommandResult(request)
	if err != nil {
		return failedIngressResult("invalid_command_result"), err
	}
	disposition, err := s.commandQueue.CommitResult(receipt.AgentID, commandID, canonicalResult)
	if err != nil {
		return failedIngressResult("command_state_conflict"), err
	}
	return s.publishQUICCommandResult(ctx, receipt, request, disposition)
}

func (s *Server) publishQUICCommandResult(
	ctx context.Context,
	receipt agentquic.IngressReceipt,
	request commandResultRequest,
	disposition command.ResultDisposition,
) (agentquic.IngressResult, error) {
	commandID := request.CommandID
	envelopeEvent, eventID := newDeterministicIngressEnvelope(receipt, "command-result")
	envelopeEvent.Payload = &pb.EventEnvelope_LogEntry{LogEntry: &pb.LogEntry{
		Level: "INFO", Component: "gateway.command.audit",
		Message: fmt.Sprintf("command_result command_id=%s type=%s status=%s hostname=%s reason=%s output_bytes=%d", commandID, clampText(request.Type, maxTypeLen), clampText(request.Status, maxStatusLen), clampText(request.ClientHostname, maxHostnameLen), clampText(request.Reason, maxReasonLen), len(request.OutputData)),
	}}
	if err := s.broker.PublishContext(ctx, "sys.in.default."+receipt.AgentID+".command.audit", envelopeEvent); err != nil {
		return failedIngressResult("broker_unavailable"), fmt.Errorf("publish QUIC command result: %w", err)
	}
	s.storeLogEnvelope(envelopeEvent)
	if err := s.recordSpecialCommandResult(receipt.AgentID, request); err != nil {
		return failedIngressResult("operation_state_conflict"), err
	}
	response := acceptedIngressResult(wire.CommitOperationTerminal, eventID)
	if disposition == command.ResultDuplicate {
		response.Status = wire.AcknowledgementDuplicate
	}
	return response, nil
}

func (s *Server) commitQUICMediaOpen(receipt agentquic.IngressReceipt, media *wire.MediaOpen) (agentquic.IngressResult, error) {
	if media == nil || s.screenSessions == nil || !s.screenSessions.AuthorizesMediaOpen(
		receipt.AgentID, media.GenerationID, media.FrameRateCap, media.MaximumFrameBytes,
	) {
		return failedIngressResult("media_not_authorized"), fmt.Errorf("commit media open: generation is not authorized")
	}
	return acceptedIngressResult(wire.CommitValidated, [16]byte{}), nil
}

func (s *Server) commitQUICMediaFrame(receipt agentquic.IngressReceipt, media *wire.MediaFrame) (agentquic.IngressResult, error) {
	if media == nil || s.screenSessions == nil || !s.screenSessions.AuthorizesMediaFrame(receipt.AgentID, media.GenerationID) {
		return failedIngressResult("media_not_authorized"), fmt.Errorf("commit media frame: generation is not authorized")
	}
	s.screenStore.Save(receipt.AgentID, ScreenFrame{
		AgentID: receipt.AgentID, CapturedAt: time.Now().UTC(), ContentType: mediaContentType(media.ContentType),
		Content: append([]byte(nil), media.Data...),
	})
	return acceptedIngressResult(wire.CommitDecoded, [16]byte{}), nil
}

func newIngressEnvelope(receipt agentquic.IngressReceipt) (*pb.EventEnvelope, [16]byte) {
	eventUUID := uuid.New()
	var eventID [16]byte
	copy(eventID[:], eventUUID[:])
	return ingressEnvelope(receipt, eventUUID.String()), eventID
}

func newDeterministicIngressEnvelope(receipt agentquic.IngressReceipt, domain string) (*pb.EventEnvelope, [16]byte) {
	hashInput := append([]byte(domain+"\x00"), receipt.OperationID[:]...)
	digest := sha256.Sum256(hashInput)
	var eventID [16]byte
	copy(eventID[:], digest[:16])
	eventID[6] = (eventID[6] & uuidVersionMask) | uuidVersionFive
	eventID[8] = (eventID[8] & uuidVariantMask) | uuidRFC4122Variant
	return ingressEnvelope(receipt, uuid.UUID(eventID).String()), eventID
}

func ingressEnvelope(receipt agentquic.IngressReceipt, eventID string) *pb.EventEnvelope {
	clientIP := receipt.RemoteAddress
	if host, _, err := net.SplitHostPort(receipt.RemoteAddress); err == nil {
		clientIP = host
	}
	return &pb.EventEnvelope{
		EventId: eventID, TraceId: uuid.UUID(receipt.TraceID).String(), Timestamp: timestamppb.Now(),
		Security: &pb.SecurityContext{
			AgentId: receipt.AgentID, SessionId: uuid.UUID(receipt.SessionID).String(),
			ClientIp: clampText(clientIP, maximumClientIPBytes), IsAuthenticated: true,
		},
	}
}

func acceptedIngressResult(commit wire.AcknowledgementCommit, receiptID [16]byte) agentquic.IngressResult {
	return agentquic.IngressResult{
		Status: wire.AcknowledgementAccepted, Commit: commit, ReceiptID: receiptID, Retry: wire.RetryNever,
	}
}

func failedIngressResult(publicCode string) agentquic.IngressResult {
	return agentquic.IngressResult{
		Status: wire.AcknowledgementFailed, Commit: wire.CommitValidated,
		Retry: wire.RetrySameOperation, PublicErrorCode: publicCode,
	}
}

func heartbeatToProtobuf(heartbeat wire.Heartbeat) *pb.Heartbeat {
	cpuCores, _ := boundedInt32FromUint64(heartbeat.CPUCores, maximumReportedCPUCores, "CPU cores")
	cpuThreads, _ := boundedInt32FromUint64(heartbeat.CPUThreads, maximumReportedCPUThreads, "CPU threads")
	applications := make([]*pb.ApplicationTypeUsage, 0, len(heartbeat.ApplicationTypes))
	for _, usage := range heartbeat.ApplicationTypes {
		applications = append(applications, &pb.ApplicationTypeUsage{Category: applicationCategory(usage.Category), Count: usage.Count})
	}
	return &pb.Heartbeat{
		Hostname: heartbeat.Hostname, OsVersion: heartbeat.OSVersion,
		CpuLoad: float64(heartbeat.CPULoadPPM) / partsPerMillion, RamUsage: float64(heartbeat.RAMUsagePPM) / partsPerMillion,
		UptimeSeconds: heartbeat.UptimeSeconds, CpuModel: heartbeat.CPUModel,
		CpuCores: cpuCores, CpuThreads: cpuThreads, TotalRamBytes: heartbeat.TotalRAMBytes,
		GpuDevices: append([]string(nil), heartbeat.GPUDevices...), NetworkName: heartbeat.NetworkName,
		NetworkAddresses: append([]string(nil), heartbeat.NetworkAddresses...), KernelVersion: heartbeat.KernelVersion,
		CpuFrequencyMhz: heartbeat.CPUFrequencyMHz, NetworkOnline: heartbeat.NetworkOnline,
		NetworkLinkSpeedMbps: heartbeat.LinkSpeedMbps, NetworkType: networkType(heartbeat.NetworkType),
		TotalStorageBytes: heartbeat.TotalStorageBytes, AvailableStorageBytes: heartbeat.AvailableStorageBytes,
		NetworkSsid: heartbeat.NetworkSSID, UsedStorageBytes: heartbeat.UsedStorageBytes,
		StorageUsage:      float64(heartbeat.StorageUsagePPM) / partsPerMillion,
		StorageInodeUsage: float64(heartbeat.InodeUsagePPM) / partsPerMillion,
		StorageDevice:     heartbeat.StorageDevice, StorageFilesystem: heartbeat.StorageFilesystem,
		StorageMountpoint: heartbeat.StorageMountpoint, StorageModel: heartbeat.StorageModel,
		StorageType: storageType(heartbeat.StorageType), StorageReadOnly: heartbeat.StorageReadOnly,
		ApplicationTypes: applications,
	}
}

func commandResultFromWire(commandID string, result wire.CommandResult) commandResultRequest {
	status := commandResultStatusRejected
	switch result.State {
	case uint64(wire.CommandResultStateExecuted):
		status = commandResultStatusExecuted
	case uint64(wire.CommandResultStateOutcomeUnknown):
		status = commandResultStatusOutcomeUnknown
	}
	return commandResultRequest{
		CommandID: commandID, Type: result.CommandType, Status: status, Reason: result.ReasonText,
		ClientHostname: result.Hostname, OutputData: append([]byte(nil), result.Result...),
		TerminalSessionID: result.TerminalSessionID, TerminalShell: result.TerminalShell,
		TerminalWorkingDirectory: result.TerminalWorkingDirectory, TerminalExitCode: int(result.TerminalExitCode),
		Result: json.RawMessage(append([]byte(nil), result.Result...)),
	}
}

func logRegistryValues(entry wire.LogEntry) (string, string, string, bool) {
	levels := map[uint64]string{1: "DEBUG", 2: "INFO", 3: "WARN", 4: "ERROR"}
	components := map[uint64]string{1: "client.runtime", 2: "client.authentication", 3: "client.attestation", 4: "client.heartbeat", 5: "client.command"}
	events := map[uint64]string{
		1: "runtime_started", 2: "authentication_succeeded", 3: "authentication_failed",
		4: "attestation_submitted", 5: "attestation_failed", 6: "heartbeat_failed",
		7: "command_received", 8: "command_completed", 9: "command_transport_failed",
		10: "command_processing_failed", 11: "command_result_submission_failed", 12: "runtime_loop_failed",
		13: "quic_network_fallback",
	}
	level, levelOK := levels[entry.Level]
	component, componentOK := components[entry.Component]
	eventName, eventOK := events[entry.EventCode]
	return level, component, eventName, levelOK && componentOK && eventOK
}

func networkType(value uint64) string {
	return map[uint64]string{1: "ethernet", 2: "wireless", 3: "loopback", 4: "other"}[value]
}

func storageType(value uint64) string {
	return map[uint64]string{1: "ssd", 2: "hdd", 3: "nvme", 4: "virtual", 5: "removable"}[value]
}

func applicationCategory(value uint16) string {
	categories := []string{"", "Browsers", "Development", "Communication", "Media", "Games", "Productivity", "Security", "Utilities and other"}
	if int(value) >= len(categories) {
		return "Utilities and other"
	}
	return categories[value]
}

func mediaContentType(value uint64) string {
	return map[uint64]string{1: "image/jpeg", 2: "image/png", 3: "video/webm"}[value]
}
