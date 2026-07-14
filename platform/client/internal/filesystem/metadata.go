package filesystem

import (
	"fmt"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

// SetMetadata applies only explicitly requested metadata deltas and reports an
// outcome for every field. Results are client-authored filesystem evidence.
func SetMetadata(request fileprotocol.MetadataSetRequest) (fileprotocol.MetadataSetResult, error) {
	if request.ProtocolVersion != fileprotocol.Version || request.OperationID == "" {
		return fileprotocol.MetadataSetResult{}, fmt.Errorf("invalid metadata protocol envelope")
	}
	components, err := validateRelativePath(request.RelativePath)
	if err != nil || len(components) == 0 {
		return fileprotocol.MetadataSetResult{}, fmt.Errorf("metadata path is invalid")
	}
	if request.Delta.ModifiedAt == nil && request.Delta.POSIXMode == nil {
		return fileprotocol.MetadataSetResult{}, fmt.Errorf("metadata delta is empty")
	}
	definition, err := resolveFilesystemRoot(request.RootID)
	if err != nil {
		return fileprotocol.MetadataSetResult{}, err
	}
	root, err := openRoot(definition.Path)
	if err != nil {
		return fileprotocol.MetadataSetResult{}, fmt.Errorf("open filesystem root: %w", err)
	}
	defer closeRootAfterRead(root)
	if err := root.checkPreconditions(components, request.Preconditions); err != nil {
		return fileprotocol.MetadataSetResult{}, err
	}
	return fileprotocol.MetadataSetResult{ProtocolVersion: fileprotocol.Version, OperationID: request.OperationID, Fields: root.setMetadata(components, request.Delta)}, nil
}
