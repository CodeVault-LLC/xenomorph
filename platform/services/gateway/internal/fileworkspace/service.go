package fileworkspace

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
	"github.com/google/uuid"
)

const (
	operationExpiry       = 2 * time.Minute
	maxRootIDBytes        = 128
	maxDirectoryPage      = 500
	maxSearchQueryBytes   = 256
	maxSearchResults      = 250
	maxSearchEntries      = 10_000
	maxSearchDepth        = 16
	maxPreviewReadBytes   = 1 << 20
	maxMutationItems      = 100
	maxAppendBytes        = 1 << 20
	maxRelativePath       = 4096
	maxPathComponents     = 256
	maxArchiveSources     = 100
	maxArchiveEntries     = 10_000
	maxArchiveDepth       = 64
	maxArchiveBytes       = 1 << 30
	maxArchiveRatio       = 100
	maxArchiveRuntime     = 30 * time.Second
	maxArchiveListItems   = 250
	maxArchiveNameBytes   = 32 << 10
	maxMetadataFutureSkew = 24 * time.Hour
)

// Service validates and dispatches durable file workspace operations.
type Service struct {
	queue     *command.Queue
	store     *Store
	transfers *TransferStore
}

// NewService creates a gateway-mediated file workspace. Filesystem roots are
// discovered and resolved by each agent.
func NewService(queue *command.Queue, store *Store) (*Service, error) {
	if queue == nil || store == nil {
		return nil, fmt.Errorf("file workspace queue and store are required")
	}
	transfers, err := newTransferStore(store.path)
	if err != nil {
		return nil, fmt.Errorf("transfer store setup: %w", err)
	}
	return &Service{queue: queue, store: store, transfers: transfers}, nil
}

// ProbeRoots persists and dispatches automatic filesystem-root discovery.
func (service *Service) ProbeRoots(agentID, operatorID, traceID string) (Operation, error) {
	now := time.Now().UTC()
	payload, err := json.Marshal(fileprotocol.RootsListRequest{ProtocolVersion: fileprotocol.Version})
	if err != nil {
		return Operation{}, fmt.Errorf("encode file root probe: %w", err)
	}
	commandID := uuid.New().String()
	operation, err := service.store.Create(Operation{
		CommandID: commandID, AgentID: agentID, OperatorID: operatorID,
		RootID: "*", Type: fileprotocol.CommandRootsList,
		ExpiresAt: now.Add(operationExpiry), AuditTraceID: traceID,
	})
	if err != nil {
		return Operation{}, err
	}
	envelope := &command.Envelope{
		CommandID: commandID, Type: fileprotocol.CommandRootsList, Payload: payload,
		RequestedBy: operatorID, Reason: "Automatic filesystem root discovery",
	}
	if err := service.queue.Enqueue(agentID, envelope); err != nil {
		_ = service.store.Fail(operation.OperationID, "dispatch_failed")
		return Operation{}, fmt.Errorf("enqueue file root probe: %w", err)
	}
	return operation, nil
}

// Dispatch validates bounded protocol input, persists state, then enqueues a
// signed command for the selected agent. The agent resolves the root ID against
// its current automatically discovered roots.
func (service *Service) Dispatch(agentID, operatorID, rootID, commandType, traceID string, request any) (Operation, error) {
	if err := validateRootID(rootID); err != nil {
		return Operation{}, err
	}
	now := time.Now().UTC()
	operationID := uuid.New().String()
	if mutation, ok := request.(*fileprotocol.MutationRequest); ok {
		mutation.OperationID = operationID
	}
	if metadata, ok := request.(*fileprotocol.MetadataSetRequest); ok {
		metadata.OperationID = operationID
	}
	if archive, ok := request.(*fileprotocol.ArchiveRequest); ok {
		archive.OperationID = operationID
	}
	if err := prepareRequest(commandType, request, rootID); err != nil {
		return Operation{}, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return Operation{}, fmt.Errorf("encode file command request: %w", err)
	}
	commandID := uuid.New().String()
	operation, err := service.store.Create(Operation{
		OperationID: operationID, CommandID: commandID,
		AgentID: agentID, OperatorID: operatorID, RootID: rootID, Type: commandType,
		ExpiresAt: now.Add(operationExpiry), AuditTraceID: traceID,
		Mutation: mutationRequest(request),
	})
	if err != nil {
		return Operation{}, err
	}
	envelope := &command.Envelope{
		CommandID: commandID,
		Type:      commandType, Payload: payload, RequestedBy: operatorID,
		Reason: fileCommandReason(commandType),
	}
	if err := service.queue.Enqueue(agentID, envelope); err != nil {
		_ = service.store.Fail(operation.OperationID, "dispatch_failed")
		return Operation{}, fmt.Errorf("enqueue file command: %w", err)
	}
	return operation, nil
}

func fileCommandReason(commandType string) string {
	if commandType == fileprotocol.CommandOperationExecute || commandType == fileprotocol.CommandMetadataSet || commandType == fileprotocol.CommandArchiveExecute {
		return "Preconditioned file workspace mutation"
	}
	return "Read-only file workspace operation"
}

// Complete records an authenticated client result for a file command.
func (service *Service) Complete(agentID, commandID, status string, result json.RawMessage) error {
	if service == nil {
		return nil
	}
	operation, err := service.store.Complete(agentID, commandID, status, result)
	if err != nil {
		return err
	}
	if status != "executed" {
		return err
	}
	switch operation.Type {
	case fileprotocol.CommandTransferPrepare, fileprotocol.CommandTransferResume, fileprotocol.CommandTransferAbort:
		return service.recordTransferResult(agentID, operation, result)
	default:
		return nil
	}
}

func (service *Service) recordTransferResult(agentID string, operation Operation, result json.RawMessage) error {
	var envelope fileprotocol.CommandResult
	if err := json.Unmarshal(result, &envelope); err != nil {
		return fmt.Errorf("decode transfer result envelope: %w", err)
	}
	var transferResult fileprotocol.TransferResult
	if err := json.Unmarshal(envelope.Data, &transferResult); err != nil {
		return fmt.Errorf("decode transfer result: %w", err)
	}
	transfer, err := service.transfers.RecordAgentResult(agentID, transferResult)
	if err != nil {
		return err
	}
	if transferResult.State == "prepared" {
		lease, err := service.transfers.IssueLease(agentID, transfer.TransferID)
		if err != nil {
			return err
		}
		return service.dispatchTransfer(agentID, operation.OperatorID, operation.AuditTraceID, transfer, lease, fileprotocol.CommandTransferResume)
	}
	return nil
}

func mutationRequest(request any) *fileprotocol.MutationRequest {
	mutation, ok := request.(*fileprotocol.MutationRequest)
	if !ok {
		return nil
	}
	copyValue := *mutation
	copyValue.Items = append([]fileprotocol.MutationItem(nil), mutation.Items...)
	return &copyValue
}

// Operation returns one agent-scoped durable operation.
func (service *Service) Operation(agentID, operationID string) (Operation, bool) {
	return service.store.Get(agentID, operationID)
}

// CreateTransfer persists a bounded manifest before any data-plane activity.
// Download transfers are dispatched immediately; uploads remain in staging
// until every browser-provided chunk passes gateway integrity verification.
func (service *Service) CreateTransfer(agentID, operatorID, traceID string, manifest fileprotocol.TransferManifest) (Transfer, error) {
	if err := validateRootID(manifest.RootID); err != nil {
		return Transfer{}, err
	}
	if err := validateOperatorRelativePath(manifest.RelativePath); err != nil {
		return Transfer{}, err
	}
	transfer, lease, err := service.transfers.CreateTransfer(agentID, operatorID, manifest)
	if err != nil {
		return Transfer{}, err
	}
	if transfer.Manifest.Direction == fileprotocol.TransferDownload {
		if err := service.dispatchTransfer(agentID, operatorID, traceID, transfer, lease, fileprotocol.CommandTransferPrepare); err != nil {
			return Transfer{}, err
		}
	}
	return transfer, nil
}

// StageTransferChunk writes and acknowledges one browser upload chunk without
// issuing a browser-side storage credential.
func (service *Service) StageTransferChunk(agentID, transferID string, index int, data []byte) (Transfer, error) {
	return service.transfers.StageBrowserChunk(agentID, transferID, index, data)
}

// CommitUpload verifies the frozen object, rotates an agent-only lease, and
// dispatches the signed local publish command.
func (service *Service) CommitUpload(agentID, operatorID, traceID, transferID string) (Transfer, error) {
	transfer, err := service.transfers.Finalize(agentID, transferID)
	if err != nil {
		return Transfer{}, err
	}
	if transfer.Manifest.Direction != fileprotocol.TransferUpload {
		return Transfer{}, fmt.Errorf("transfer is not an upload")
	}
	lease, err := service.transfers.IssueLease(agentID, transferID)
	if err != nil {
		return Transfer{}, err
	}
	if err := service.dispatchTransfer(agentID, operatorID, traceID, transfer, lease, fileprotocol.CommandTransferPrepare); err != nil {
		return Transfer{}, err
	}
	return transfer, nil
}

// Transfer returns one agent-scoped durable transfer.
func (service *Service) Transfer(agentID, transferID string) (Transfer, bool) {
	return service.transfers.Transfer(agentID, transferID)
}

// Transfers returns durable transfer state for the dashboard drawer.
func (service *Service) Transfers(agentID string) []Transfer {
	return service.transfers.Transfers(agentID)
}

// RemoveTransfer audits and removes one terminal gateway transfer record and
// its encrypted staging bytes. It does not affect the source or saved file.
func (service *Service) RemoveTransfer(agentID, operatorID, traceID, transferID string) error {
	transfer, ok := service.transfers.Transfer(agentID, transferID)
	if !ok {
		return fmt.Errorf("transfer not found")
	}
	if !terminalTransferState(transfer.State) {
		return fmt.Errorf("active transfer cannot be removed")
	}
	if err := service.store.appendTransferRemovalAudit(agentID, operatorID, traceID, transferID, transfer.Manifest.RootID, "transfer_removal_requested"); err != nil {
		return err
	}
	return service.transfers.Remove(agentID, transferID)
}

// RemoveFinishedTransfers audits and removes every terminal gateway transfer
// for one agent. Active and other-agent transfers are retained.
func (service *Service) RemoveFinishedTransfers(agentID, operatorID, traceID string) (int, error) {
	finished := 0
	for _, transfer := range service.transfers.Transfers(agentID) {
		if terminalTransferState(transfer.State) {
			finished++
		}
	}
	if finished == 0 {
		return 0, nil
	}
	if err := service.store.appendTransferRemovalAudit(agentID, operatorID, traceID, "", "*", "finished_transfers_removal_requested"); err != nil {
		return 0, err
	}
	return service.transfers.RemoveFinished(agentID)
}

// ResumeTransfer rotates the expired lease and dispatches a signed resume.
func (service *Service) ResumeTransfer(agentID, operatorID, traceID, transferID string) (Transfer, error) {
	transfer, ok := service.transfers.Transfer(agentID, transferID)
	if !ok || transfer.State != TransferPaused && transfer.State != TransferQueued {
		return Transfer{}, fmt.Errorf("transfer is not resumable")
	}
	lease, err := service.transfers.IssueLease(agentID, transferID)
	if err != nil {
		return Transfer{}, err
	}
	if err := service.dispatchTransfer(agentID, operatorID, traceID, transfer, lease, fileprotocol.CommandTransferResume); err != nil {
		return Transfer{}, err
	}
	return transfer, nil
}

// AbortTransfer dispatches an idempotent signed cancellation command.
func (service *Service) AbortTransfer(agentID, operatorID, traceID, transferID string) (Transfer, error) {
	transfer, ok := service.transfers.Transfer(agentID, transferID)
	if !ok || transfer.State == TransferCompleted || transfer.State == TransferCancelled {
		return Transfer{}, fmt.Errorf("transfer is not cancellable")
	}
	lease, err := service.transfers.IssueLease(agentID, transferID)
	if err != nil {
		return Transfer{}, err
	}
	if err := service.dispatchTransfer(agentID, operatorID, traceID, transfer, lease, fileprotocol.CommandTransferAbort); err != nil {
		return Transfer{}, err
	}
	return transfer, nil
}

// PutAgentTransferChunk verifies a scoped chunk from the authenticated agent.
func (service *Service) PutAgentTransferChunk(agentID, transferID, token string, index int, data []byte) (Transfer, error) {
	return service.transfers.PutChunk(agentID, transferID, token, index, data)
}

// ReadAgentTransferChunk returns a scoped staged chunk to the authenticated agent.
func (service *Service) ReadAgentTransferChunk(agentID, transferID, token string, index int) ([]byte, error) {
	return service.transfers.ReadChunk(agentID, transferID, token, index)
}

// ReadCompletedTransferChunk returns verified bytes to the dashboard transport.
func (service *Service) ReadCompletedTransferChunk(agentID, transferID string, index int) ([]byte, error) {
	return service.transfers.ReadCompletedChunk(agentID, transferID, index)
}

// FinalizeAgentTransfer verifies the scoped lease and complete staged object.
func (service *Service) FinalizeAgentTransfer(agentID, transferID, token string) (Transfer, error) {
	if err := service.transfers.ValidateLease(agentID, transferID, token); err != nil {
		return Transfer{}, err
	}
	return service.transfers.Finalize(agentID, transferID)
}

func (service *Service) dispatchTransfer(agentID, operatorID, traceID string, transfer Transfer, lease fileprotocol.DataPlaneLease, commandType string) error {
	request := fileprotocol.TransferRequest{ProtocolVersion: fileprotocol.Version, Manifest: transfer.Manifest, Lease: lease}
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode transfer request: %w", err)
	}
	commandID := uuid.New().String()
	operation, err := service.store.Create(Operation{
		CommandID: commandID, TransferID: transfer.TransferID, AgentID: agentID, OperatorID: operatorID,
		RootID: transfer.Manifest.RootID, Type: commandType,
		ExpiresAt: lease.ExpiresAt, AuditTraceID: traceID,
	})
	if err != nil {
		return err
	}
	if err := service.queue.Enqueue(agentID, &command.Envelope{
		CommandID: commandID, Type: commandType, Payload: payload,
		RequestedBy: operatorID, Reason: "Staged file transfer",
	}); err != nil {
		_ = service.store.Fail(operation.OperationID, "dispatch_failed")
		return fmt.Errorf("enqueue transfer command: %w", err)
	}
	return nil
}

func prepareRequest(commandType string, request any, rootID string) error {
	switch typed := request.(type) {
	case *fileprotocol.DirectoryListRequest:
		return prepareDirectoryRequest(commandType, rootID, typed)
	case *fileprotocol.DirectorySearchRequest:
		return prepareDirectorySearchRequest(commandType, rootID, typed)
	case *fileprotocol.MetadataGetRequest:
		return prepareMetadataRequest(commandType, rootID, typed)
	case *fileprotocol.MetadataSetRequest:
		return prepareMetadataSetRequest(commandType, rootID, typed)
	case *fileprotocol.ArchiveRequest:
		return prepareArchiveRequest(commandType, rootID, typed)
	case *fileprotocol.PreviewReadRequest:
		return preparePreviewRequest(commandType, rootID, typed)
	case *fileprotocol.MutationRequest:
		return prepareMutationRequest(commandType, rootID, typed)
	default:
		return fmt.Errorf("unsupported file command request")
	}
}

func prepareDirectorySearchRequest(commandType, rootID string, request *fileprotocol.DirectorySearchRequest) error {
	if commandType != fileprotocol.CommandDirectorySearch || !validDirectorySearchQuery(request.Query) {
		return fmt.Errorf("invalid directory search request")
	}
	if !validDirectorySearchBounds(request) {
		return fmt.Errorf("directory search bounds exceed limit")
	}
	if request.RelativePath != "" {
		if err := validateOperatorRelativePath(request.RelativePath); err != nil {
			return fmt.Errorf("directory search path is invalid")
		}
	}
	request.ProtocolVersion, request.RootID = fileprotocol.Version, rootID
	return nil
}

func validDirectorySearchQuery(query string) bool {
	return strings.TrimSpace(query) == query && len(query) >= 2 && len(query) <= maxSearchQueryBytes && utf8.ValidString(query)
}

func validDirectorySearchBounds(request *fileprotocol.DirectorySearchRequest) bool {
	return request.MaxResults > 0 && request.MaxResults <= maxSearchResults &&
		request.MaxEntries > 0 && request.MaxEntries <= maxSearchEntries &&
		request.MaxDepth >= 0 && request.MaxDepth <= maxSearchDepth
}

func prepareDirectoryRequest(commandType, rootID string, request *fileprotocol.DirectoryListRequest) error {
	if commandType != fileprotocol.CommandDirectoryList || request.PageSize <= 0 || request.PageSize > maxDirectoryPage {
		return fmt.Errorf("invalid directory request")
	}
	request.ProtocolVersion, request.RootID = fileprotocol.Version, rootID
	return nil
}
func prepareMetadataRequest(commandType, rootID string, request *fileprotocol.MetadataGetRequest) error {
	if commandType != fileprotocol.CommandMetadataGet {
		return fmt.Errorf("invalid metadata request")
	}
	request.ProtocolVersion, request.RootID = fileprotocol.Version, rootID
	return nil
}
func prepareMetadataSetRequest(commandType, rootID string, request *fileprotocol.MetadataSetRequest) error {
	if commandType != fileprotocol.CommandMetadataSet || request.Delta.ModifiedAt == nil && request.Delta.POSIXMode == nil {
		return fmt.Errorf("invalid metadata update request")
	}
	if err := validateOperatorRelativePath(request.RelativePath); err != nil {
		return fmt.Errorf("metadata path is invalid")
	}
	if request.Delta.ModifiedAt != nil {
		minimum, maximum := time.Unix(0, 0).UTC(), time.Now().UTC().Add(maxMetadataFutureSkew)
		if request.Delta.ModifiedAt.Before(minimum) || request.Delta.ModifiedAt.After(maximum) {
			return fmt.Errorf("metadata timestamp is outside limit")
		}
	}
	if request.Delta.POSIXMode != nil && *request.Delta.POSIXMode > 0o7777 {
		return fmt.Errorf("POSIX mode is outside limit")
	}
	request.ProtocolVersion, request.RootID = fileprotocol.Version, rootID
	return nil
}
func prepareArchiveRequest(commandType, rootID string, request *fileprotocol.ArchiveRequest) error {
	if commandType != fileprotocol.CommandArchiveExecute || request.Format != fileprotocol.ArchiveZIP || !validArchiveAction(request.Action) {
		return fmt.Errorf("invalid archive request")
	}
	if err := validateOperatorRelativePath(request.ArchivePath); err != nil {
		return fmt.Errorf("archive path is invalid")
	}
	if err := validateArchiveOperands(request); err != nil {
		return err
	}
	if request.Action != fileprotocol.ArchiveList && !validArchiveConflict(request.Conflict) {
		return fmt.Errorf("archive conflict strategy is invalid")
	}
	request.ProtocolVersion, request.RootID = fileprotocol.Version, rootID
	request.Limits = fileprotocol.ArchiveLimits{
		MaxEntries: maxArchiveEntries, MaxDepth: maxArchiveDepth,
		MaxExpandedBytes: maxArchiveBytes, MaxTemporaryBytes: maxArchiveBytes,
		MaxCompressionRatio: maxArchiveRatio, MaxRuntime: maxArchiveRuntime,
		MaxListedEntries: maxArchiveListItems, MaxListedNameBytes: maxArchiveNameBytes,
	}
	return nil
}

func validArchiveAction(action fileprotocol.ArchiveAction) bool {
	return action == fileprotocol.ArchiveCreate || action == fileprotocol.ArchiveList || action == fileprotocol.ArchiveExtract
}

func validateArchiveOperands(request *fileprotocol.ArchiveRequest) error {
	if request.Action == fileprotocol.ArchiveCreate {
		return validateArchiveSources(request.SourcePaths)
	}
	if request.Action == fileprotocol.ArchiveExtract {
		if err := validateOperatorRelativePath(request.DestinationPath); err != nil {
			return fmt.Errorf("archive destination is invalid")
		}
	}
	return nil
}

func validateArchiveSources(sources []string) error {
	if len(sources) == 0 || len(sources) > maxArchiveSources {
		return fmt.Errorf("archive source count exceeds limit")
	}
	for _, source := range sources {
		if err := validateOperatorRelativePath(source); err != nil {
			return fmt.Errorf("archive source path is invalid")
		}
	}
	return nil
}

func validArchiveConflict(conflict fileprotocol.ConflictStrategy) bool {
	return conflict == fileprotocol.ConflictFail || conflict == fileprotocol.ConflictSkip || conflict == fileprotocol.ConflictRenameNew
}
func preparePreviewRequest(commandType, rootID string, request *fileprotocol.PreviewReadRequest) error {
	if commandType != fileprotocol.CommandPreviewRead || request.Offset < 0 || request.Length <= 0 || request.Length > maxPreviewReadBytes {
		return fmt.Errorf("invalid preview request")
	}
	request.ProtocolVersion, request.RootID = fileprotocol.Version, rootID
	return nil
}
func prepareMutationRequest(commandType, rootID string, request *fileprotocol.MutationRequest) error {
	if commandType != fileprotocol.CommandOperationExecute || !validMutationVerb(request.Verb) || !validConflictStrategy(request.Conflict) {
		return fmt.Errorf("invalid mutation request")
	}
	if len(request.Items) == 0 || len(request.Items) > maxMutationItems {
		return fmt.Errorf("mutation item count exceeds limit")
	}
	for _, item := range request.Items {
		if err := validateMutationItem(request.Verb, item); err != nil {
			return err
		}
	}
	request.ProtocolVersion, request.RootID = fileprotocol.Version, rootID
	return nil
}

func validateMutationItem(verb fileprotocol.MutationVerb, item fileprotocol.MutationItem) error {
	if item.ItemID == "" || len(item.AppendData) > maxAppendBytes || item.TruncateSize < 0 {
		return fmt.Errorf("mutation item is invalid")
	}
	if mutationNeedsSource(verb) {
		if err := validateOperatorRelativePath(item.SourcePath); err != nil {
			return fmt.Errorf("mutation source path is invalid")
		}
	}
	if mutationNeedsDestination(verb) {
		if err := validateOperatorRelativePath(item.DestinationPath); err != nil {
			return fmt.Errorf("mutation destination path is invalid")
		}
	}
	return nil
}

func mutationNeedsSource(verb fileprotocol.MutationVerb) bool {
	return verb != fileprotocol.MutationCreateFile && verb != fileprotocol.MutationCreateDirectory
}

func mutationNeedsDestination(verb fileprotocol.MutationVerb) bool {
	switch verb {
	case fileprotocol.MutationCreateFile, fileprotocol.MutationCreateDirectory,
		fileprotocol.MutationRename, fileprotocol.MutationMove, fileprotocol.MutationCopy,
		fileprotocol.MutationDuplicate:
		return true
	default:
		return false
	}
}

func validateOperatorRelativePath(value string) error {
	if value == "" {
		return fmt.Errorf("relative path is required")
	}
	if err := validateOperatorPathEncoding(value); err != nil {
		return err
	}
	return validateOperatorPathComponents(strings.Split(value, "/"))
}

func validateOperatorPathEncoding(value string) error {
	if len(value) > maxRelativePath || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\\\r\n") {
		return fmt.Errorf("relative path encoding is invalid")
	}
	if strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return fmt.Errorf("relative path is not normalized")
	}
	return nil
}

func validateOperatorPathComponents(components []string) error {
	if len(components) > maxPathComponents {
		return fmt.Errorf("relative path depth exceeds limit")
	}
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("relative path contains a forbidden component")
		}
	}
	return nil
}

func validMutationVerb(verb fileprotocol.MutationVerb) bool {
	switch verb {
	case fileprotocol.MutationCreateFile, fileprotocol.MutationCreateDirectory,
		fileprotocol.MutationRename, fileprotocol.MutationMove, fileprotocol.MutationCopy,
		fileprotocol.MutationDuplicate, fileprotocol.MutationTouch,
		fileprotocol.MutationTruncate, fileprotocol.MutationAppend,
		fileprotocol.MutationDelete:
		return true
	default:
		return false
	}
}

func validConflictStrategy(strategy fileprotocol.ConflictStrategy) bool {
	switch strategy {
	case fileprotocol.ConflictFail, fileprotocol.ConflictSkip, fileprotocol.ConflictRenameNew, fileprotocol.ConflictReplace:
		return true
	default:
		return false
	}
}

func validateRootID(rootID string) error {
	if rootID == "" || len(rootID) > maxRootIDBytes || !utf8.ValidString(rootID) || strings.TrimSpace(rootID) != rootID {
		return fmt.Errorf("filesystem root ID is invalid")
	}
	for _, character := range rootID {
		if !validRootIDCharacter(character) {
			return fmt.Errorf("filesystem root ID is invalid")
		}
	}
	return nil
}

func validRootIDCharacter(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-'
}
