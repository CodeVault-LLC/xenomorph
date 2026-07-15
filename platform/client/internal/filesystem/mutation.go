package filesystem

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

// ExecuteMutation plans or executes bounded mutations under one currently
// discovered root. Every result is client-authored operation evidence.
func ExecuteMutation(request fileprotocol.MutationRequest) (fileprotocol.MutationResult, error) {
	if request.ProtocolVersion != fileprotocol.Version || request.OperationID == "" {
		return fileprotocol.MutationResult{}, fmt.Errorf("invalid mutation protocol envelope")
	}

	if len(request.Items) == 0 || len(request.Items) > maxMutationItems {
		return fileprotocol.MutationResult{}, fmt.Errorf("mutation item count exceeds limit")
	}

	definition, err := resolveFilesystemRoot(request.RootID)
	if err != nil {
		return fileprotocol.MutationResult{}, err
	}

	root, err := openRoot(definition.Path)
	if err != nil {
		return fileprotocol.MutationResult{}, fmt.Errorf("open filesystem root: %w", err)
	}

	defer closeRootAfterRead(root)

	result := fileprotocol.MutationResult{
		ProtocolVersion: fileprotocol.Version, OperationID: request.OperationID,
		DryRun: request.DryRun, Items: make([]fileprotocol.ItemResult, 0, len(request.Items)),
	}
	for _, item := range request.Items {
		result.Items = append(result.Items, executeMutationItem(root, request, item))
	}

	return result, nil
}

func executeMutationItem(root *rootHandle, request fileprotocol.MutationRequest, item fileprotocol.MutationItem) fileprotocol.ItemResult {
	if item.ItemID == "" {
		return fileprotocol.ItemResult{State: "failed", ErrorClass: "invalid_input"}
	}

	result, err := root.mutate(request.Verb, request.Conflict, request.DryRun, item)
	if err != nil {
		return fileprotocol.ItemResult{ItemID: item.ItemID, State: "failed", ErrorClass: mutationErrorClass(err)}
	}

	result.ItemID = item.ItemID
	if request.DryRun {
		result.State = "planned"
	}

	return result
}

const maxMutationItems = 100

const (
	maxRecursiveDeleteEntries = 10_000
	maxRecursiveDeleteDepth   = 64
	deleteScanBatchSize       = 128
)

func mutationErrorClass(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "not_found"
	case errors.Is(err, fs.ErrPermission):
		return "forbidden"
	case errors.Is(err, fs.ErrExist):
		return "conflict"
	default:
		return "permanent_failure"
	}
}

func hasPreconditions(value fileprotocol.Preconditions) bool {
	return value.MustExist != nil || value.ExpectedKind != "" || value.ExpectedSize != nil || !value.ExpectedModTime.IsZero()
}
