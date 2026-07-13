package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"

	clientfs "github.com/codevault-llc/xenomorph/platform/client/internal/filesystem"
	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func executeFileCommand(commandType CommandType, payload json.RawMessage) commandOutcome {
	result, err := runFileCommand(commandType, payload)
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

func runFileCommand(commandType CommandType, payload json.RawMessage) (any, error) {
	switch commandType {
	case CommandTypeFilesRootsList:
		var request fileprotocol.RootsListRequest
		if err := decodeFileRequest(payload, &request); err != nil {
			return nil, err
		}
		return clientfs.ListRoots(request)
	case CommandTypeFilesDirectoryList:
		var request fileprotocol.DirectoryListRequest
		if err := decodeFileRequest(payload, &request); err != nil {
			return nil, err
		}
		return clientfs.ListDirectory(request)
	case CommandTypeFilesMetadataGet:
		var request fileprotocol.MetadataGetRequest
		if err := decodeFileRequest(payload, &request); err != nil {
			return nil, err
		}
		return clientfs.GetMetadata(request)
	case CommandTypeFilesPreviewRead:
		var request fileprotocol.PreviewReadRequest
		if err := decodeFileRequest(payload, &request); err != nil {
			return nil, err
		}
		return clientfs.ReadPreview(request)
	default:
		return nil, fmt.Errorf("unsupported file command")
	}
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
