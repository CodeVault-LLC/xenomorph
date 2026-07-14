package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"

	clientfs "github.com/codevault-llc/xenomorph/platform/client/internal/filesystem"
	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func executeFileCommand(ctx context.Context, commandType CommandType, payload json.RawMessage, plane clientfs.TransferPlane) commandOutcome {
	result, err := runFileCommand(ctx, commandType, payload, plane)
	if err != nil {
		return fileCommandError(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fileCommandError(fmt.Errorf("encode file command result: %w", err))
	}
	wrapped, err := json.Marshal(fileprotocol.CommandResult{ProtocolVersion: fileprotocol.Version, Data: data})
	if err != nil {
		return fileCommandError(fmt.Errorf("encode file result envelope: %w", err))
	}
	return commandOutcome{reason: "file operation completed", resultData: wrapped}
}

func runFileCommand(ctx context.Context, commandType CommandType, payload json.RawMessage, plane clientfs.TransferPlane) (any, error) {
	switch commandType {
	case CommandTypeFilesRootsList:
		return runRootsList(payload)
	case CommandTypeFilesDirectoryList:
		return runDirectoryList(payload)
	case CommandTypeFilesDirectorySearch:
		return runDirectorySearch(ctx, payload)
	case CommandTypeFilesMetadataGet:
		return runMetadataGet(payload)
	case CommandTypeFilesMetadataSet:
		return runMetadataSet(payload)
	case CommandTypeFilesArchiveExecute:
		return runArchive(ctx, payload)
	case CommandTypeFilesPreviewRead:
		return runPreviewRead(payload)
	default:
		return runFileMutationCommand(ctx, commandType, payload, plane)
	}
}

func runFileMutationCommand(ctx context.Context, commandType CommandType, payload json.RawMessage, plane clientfs.TransferPlane) (any, error) {
	switch commandType {
	case CommandTypeFilesOperationExecute:
		return runMutation(payload)
	case CommandTypeFilesTransferPrepare, CommandTypeFilesTransferResume:
		return runTransfer(ctx, commandType, payload, plane)
	case CommandTypeFilesTransferAbort:
		return runTransferAbort(payload)
	default:
		return nil, fmt.Errorf("unsupported file command")
	}
}

func runRootsList(payload json.RawMessage) (any, error) {
	var request fileprotocol.RootsListRequest
	if err := decodeFileRequest(payload, &request); err != nil {
		return nil, err
	}
	return clientfs.ListRoots(request)
}
func runDirectoryList(payload json.RawMessage) (any, error) {
	var request fileprotocol.DirectoryListRequest
	if err := decodeFileRequest(payload, &request); err != nil {
		return nil, err
	}
	return clientfs.ListDirectory(request)
}
func runDirectorySearch(ctx context.Context, payload json.RawMessage) (any, error) {
	var request fileprotocol.DirectorySearchRequest
	if err := decodeFileRequest(payload, &request); err != nil {
		return nil, err
	}
	return clientfs.SearchDirectory(ctx, request)
}
func runMetadataGet(payload json.RawMessage) (any, error) {
	var request fileprotocol.MetadataGetRequest
	if err := decodeFileRequest(payload, &request); err != nil {
		return nil, err
	}
	return clientfs.GetMetadata(request)
}
func runMetadataSet(payload json.RawMessage) (any, error) {
	var request fileprotocol.MetadataSetRequest
	if err := decodeFileRequest(payload, &request); err != nil {
		return nil, err
	}
	return clientfs.SetMetadata(request)
}
func runArchive(ctx context.Context, payload json.RawMessage) (any, error) {
	var request fileprotocol.ArchiveRequest
	if err := decodeFileRequest(payload, &request); err != nil {
		return nil, err
	}
	return clientfs.ExecuteArchive(ctx, request)
}
func runPreviewRead(payload json.RawMessage) (any, error) {
	var request fileprotocol.PreviewReadRequest
	if err := decodeFileRequest(payload, &request); err != nil {
		return nil, err
	}
	return clientfs.ReadPreview(request)
}
func runMutation(payload json.RawMessage) (any, error) {
	var request fileprotocol.MutationRequest
	if err := decodeFileRequest(payload, &request); err != nil {
		return nil, err
	}
	return clientfs.ExecuteMutation(request)
}

func runTransfer(ctx context.Context, commandType CommandType, payload json.RawMessage, plane clientfs.TransferPlane) (any, error) {
	var request fileprotocol.TransferRequest
	if err := decodeFileRequest(payload, &request); err != nil {
		return nil, err
	}
	if commandType == CommandTypeFilesTransferPrepare && request.Manifest.Direction == fileprotocol.TransferDownload && len(request.Manifest.Chunks) == 0 {
		result, err := clientfs.PrepareDownload(request)
		if err != nil {
			return clientfs.TransferFailureResult(request, err), nil
		}
		return result, nil
	}
	result, err := clientfs.ExecuteTransfer(ctx, request, plane)
	if err != nil {
		return clientfs.TransferFailureResult(request, err), nil
	}
	return result, nil
}

func runTransferAbort(payload json.RawMessage) (any, error) {
	var request fileprotocol.TransferRequest
	if err := decodeFileRequest(payload, &request); err != nil {
		return nil, err
	}
	return fileprotocol.TransferResult{ProtocolVersion: fileprotocol.Version, TransferID: request.Manifest.TransferID, State: "cancelled", Scanning: "not_scanned"}, nil
}

func decodeFileRequest(payload json.RawMessage, destination any) error {
	if len(payload) == 0 {
		return fmt.Errorf("file command payload is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode file command payload: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("file command payload contains trailing data")
	}
	return nil
}

func fileCommandError(err error) commandOutcome {
	class := "permanent_failure"
	if errors.Is(err, fs.ErrNotExist) {
		class = "not_found"
	} else if errors.Is(err, fs.ErrPermission) {
		class = "forbidden"
	}
	wrapped, marshalErr := json.Marshal(fileprotocol.CommandResult{
		ProtocolVersion: fileprotocol.Version,
		Error:           &fileprotocol.Error{Class: class, Message: "filesystem operation could not be completed"},
	})
	if marshalErr != nil {
		return commandOutcome{reason: "file operation failed"}
	}
	return commandOutcome{reason: "file operation failed", resultData: wrapped}
}
