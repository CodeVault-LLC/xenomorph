//go:build windows

package filesystem

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func (root *rootHandle) applyUpload(ctx context.Context, request fileprotocol.TransferRequest, plane TransferPlane) (fileprotocol.TransferResult, error) {
	if request.Manifest.Conflict == fileprotocol.ConflictReplace {
		return fileprotocol.TransferResult{}, fmt.Errorf("atomic replacement is unavailable on this platform adapter")
	}
	components, err := validateRelativePath(request.Manifest.RelativePath)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}
	if hasPreconditions(request.Manifest.Preconditions) {
		if err := root.checkPreconditions(components, request.Manifest.Preconditions); err != nil {
			return fileprotocol.TransferResult{}, err
		}
	}
	destination := filepath.Join(append([]string{root.path}, components...)...)
	identifier, err := randomMutationID()
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}
	temporary := destination + ".xenomorph-" + identifier
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}
	published := false
	defer func() {
		if !published {
			_ = os.Remove(temporary)
		}
	}()
	whole := sha256.New()
	acknowledged := make([]int, 0, len(request.Manifest.Chunks))
	for _, chunk := range request.Manifest.Chunks {
		var data []byte
		if err := retryTransfer(ctx, func() error {
			var getErr error
			data, getErr = plane.GetChunk(ctx, request.Manifest.TransferID, request.Lease.Token, chunk.Index, chunk.Size)
			return getErr
		}); err != nil {
			_ = file.Close()
			return fileprotocol.TransferResult{}, err
		}
		if int64(len(data)) != chunk.Size || transferDigest(data) != chunk.SHA256 {
			_ = file.Close()
			return fileprotocol.TransferResult{}, fmt.Errorf("staged chunk integrity mismatch")
		}
		if _, err := file.Write(data); err != nil {
			_ = file.Close()
			return fileprotocol.TransferResult{}, err
		}
		if _, err := whole.Write(data); err != nil {
			_ = file.Close()
			return fileprotocol.TransferResult{}, err
		}
		acknowledged = append(acknowledged, chunk.Index)
	}
	if hex.EncodeToString(whole.Sum(nil)) != request.Manifest.SHA256 {
		_ = file.Close()
		return fileprotocol.TransferResult{}, fmt.Errorf("staged object integrity mismatch")
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fileprotocol.TransferResult{}, err
	}
	if err := file.Close(); err != nil {
		return fileprotocol.TransferResult{}, err
	}
	if _, err := os.Lstat(destination); err == nil {
		switch request.Manifest.Conflict {
		case fileprotocol.ConflictSkip:
			return fileprotocol.TransferResult{ProtocolVersion: fileprotocol.Version, TransferID: request.Manifest.TransferID, State: "skipped", Scanning: "not_scanned"}, nil
		case fileprotocol.ConflictFail:
			return fileprotocol.TransferResult{}, os.ErrExist
		case fileprotocol.ConflictRenameNew:
			destination = availableWindowsName(destination)
		case fileprotocol.ConflictReplace:
			if err := os.Remove(destination); err != nil {
				return fileprotocol.TransferResult{}, err
			}
		}
	}
	if err := os.Rename(temporary, destination); err != nil {
		return fileprotocol.TransferResult{}, err
	}
	published = true
	return fileprotocol.TransferResult{ProtocolVersion: fileprotocol.Version, TransferID: request.Manifest.TransferID, State: "completed", Acknowledged: acknowledged, BytesVerified: request.Manifest.Size, Scanning: "not_scanned"}, nil
}

func (root *rootHandle) mutate(verb fileprotocol.MutationVerb, conflict fileprotocol.ConflictStrategy, dryRun bool, item fileprotocol.MutationItem) (fileprotocol.ItemResult, error) {
	if conflict == fileprotocol.ConflictReplace && verbNeedsDestination(verb) {
		return fileprotocol.ItemResult{}, fmt.Errorf("atomic replacement is unavailable on this platform adapter")
	}
	source, sourceErr := validateRelativePath(item.SourcePath)
	destination, destinationErr := validateRelativePath(item.DestinationPath)
	if verbNeedsSource(verb) && (sourceErr != nil || len(source) == 0) {
		return fileprotocol.ItemResult{}, fmt.Errorf("mutation source is invalid")
	}
	if verbNeedsDestination(verb) && (destinationErr != nil || len(destination) == 0) {
		return fileprotocol.ItemResult{}, fmt.Errorf("mutation destination is invalid")
	}
	if verbNeedsSource(verb) {
		if err := root.checkPreconditions(source, item.Preconditions); err != nil {
			return fileprotocol.ItemResult{}, err
		}
	}
	sourcePath := filepath.Join(append([]string{root.path}, source...)...)
	destinationPath := filepath.Join(append([]string{root.path}, destination...)...)
	if dryRun {
		if verb == fileprotocol.MutationDelete {
			_, err := buildDeletePlanWindows(sourcePath, maxRecursiveDeleteEntries)
			return fileprotocol.ItemResult{State: "planned"}, err
		}
		return fileprotocol.ItemResult{State: "planned"}, validateMutationParent(root, destination, verbNeedsDestination(verb))
	}
	switch verb {
	case fileprotocol.MutationCreateFile:
		file, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fileprotocol.ItemResult{}, err
		}
		return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(destination)}, file.Close()
	case fileprotocol.MutationCreateDirectory:
		return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(destination)}, os.Mkdir(destinationPath, 0o700)
	case fileprotocol.MutationTouch:
		now := time.Now()
		return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(source)}, os.Chtimes(sourcePath, now, now)
	case fileprotocol.MutationTruncate:
		return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(source), BytesApplied: item.TruncateSize}, os.Truncate(sourcePath, item.TruncateSize)
	case fileprotocol.MutationAppend:
		return appendWindows(sourcePath, source, item.AppendData)
	case fileprotocol.MutationRename, fileprotocol.MutationMove:
		return renameWindows(sourcePath, destinationPath, destination, conflict)
	case fileprotocol.MutationCopy, fileprotocol.MutationDuplicate:
		return copyWindows(sourcePath, destinationPath, destination, conflict)
	case fileprotocol.MutationDelete:
		return deleteWindows(sourcePath)
	default:
		return fileprotocol.ItemResult{}, fmt.Errorf("mutation is not supported")
	}
}

type windowsDeletePlanEntry struct {
	path string
	info os.FileInfo
}

func buildDeletePlanWindows(path string, limit int) ([]windowsDeletePlanEntry, error) {
	plan := make([]windowsDeletePlanEntry, 0, 1)
	err := filepath.WalkDir(path, func(candidate string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if len(plan) >= limit {
			return fmt.Errorf("recursive deletion exceeds the %d-entry limit", limit)
		}
		relative, err := filepath.Rel(path, candidate)
		if err != nil {
			return err
		}
		if relative != "." && len(strings.Split(relative, string(filepath.Separator))) > maxRecursiveDeleteDepth {
			return fmt.Errorf("recursive deletion exceeds the %d-level limit", maxRecursiveDeleteDepth)
		}
		info, err := os.Lstat(candidate)
		if err != nil {
			return err
		}
		plan = append(plan, windowsDeletePlanEntry{path: candidate, info: info})
		return nil
	})
	return plan, err
}

func deleteWindows(path string) (fileprotocol.ItemResult, error) {
	plan, err := buildDeletePlanWindows(path, maxRecursiveDeleteEntries)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}
	for index := len(plan) - 1; index >= 0; index-- {
		current, statErr := os.Lstat(plan[index].path)
		if statErr != nil {
			return fileprotocol.ItemResult{}, statErr
		}
		if !os.SameFile(plan[index].info, current) || plan[index].info.Mode().Type() != current.Mode().Type() {
			return fileprotocol.ItemResult{}, fsConflict("entry changed during recursive deletion")
		}
		if removeErr := os.Remove(plan[index].path); removeErr != nil {
			return fileprotocol.ItemResult{}, removeErr
		}
	}
	return fileprotocol.ItemResult{State: "completed"}, nil
}

func (root *rootHandle) checkPreconditions(components []string, expected fileprotocol.Preconditions) error {
	_, info, err := root.resolveNoFollow(components)
	if expected.MustExist != nil && !*expected.MustExist {
		if err == nil {
			return fsConflict("target exists")
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	if err != nil {
		return err
	}
	if expected.ExpectedKind != "" && kindFromMode(info.Mode()) != expected.ExpectedKind {
		return fsConflict("target kind changed")
	}
	if expected.ExpectedSize != nil && info.Size() != *expected.ExpectedSize {
		return fsConflict("target size changed")
	}
	if !expected.ExpectedModTime.IsZero() && !info.ModTime().UTC().Equal(expected.ExpectedModTime.UTC()) {
		return fsConflict("target modification time changed")
	}
	return nil
}

func validateMutationParent(root *rootHandle, destination []string, required bool) error {
	if !required {
		return nil
	}
	_, info, err := root.resolveNoFollow(destination[:len(destination)-1])
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("mutation parent is not a directory")
	}
	return nil
}

func appendWindows(path string, components []string, data []byte) (fileprotocol.ItemResult, error) {
	if len(data) > 1<<20 {
		return fileprotocol.ItemResult{}, fmt.Errorf("append data exceeds limit")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}
	defer func() { _ = file.Close() }()
	written, err := file.Write(data)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}
	if written != len(data) {
		return fileprotocol.ItemResult{}, io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return fileprotocol.ItemResult{}, err
	}
	return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(components), BytesApplied: int64(written)}, nil
}

func renameWindows(source, destination string, components []string, conflict fileprotocol.ConflictStrategy) (fileprotocol.ItemResult, error) {
	if _, err := os.Lstat(destination); err == nil {
		switch conflict {
		case fileprotocol.ConflictSkip:
			return fileprotocol.ItemResult{State: "skipped", ResultPath: joinRelative(components)}, nil
		case fileprotocol.ConflictFail:
			return fileprotocol.ItemResult{}, os.ErrExist
		case fileprotocol.ConflictRenameNew:
			destination = availableWindowsName(destination)
			components[len(components)-1] = filepath.Base(destination)
		case fileprotocol.ConflictReplace:
			if err := os.Remove(destination); err != nil {
				return fileprotocol.ItemResult{}, err
			}
		}
	}
	if err := os.Rename(source, destination); err != nil {
		return fileprotocol.ItemResult{}, err
	}
	return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(components)}, nil
}

func copyWindows(source, destination string, components []string, conflict fileprotocol.ConflictStrategy) (fileprotocol.ItemResult, error) {
	input, err := os.Open(source)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}
	defer func() { _ = input.Close() }()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fileprotocol.ItemResult{}, fmt.Errorf("copy source is not a regular file")
	}
	if _, err := os.Lstat(destination); err == nil {
		if conflict == fileprotocol.ConflictSkip {
			return fileprotocol.ItemResult{State: "skipped", ResultPath: joinRelative(components)}, nil
		}
		if conflict == fileprotocol.ConflictFail {
			return fileprotocol.ItemResult{}, os.ErrExist
		}
		if conflict == fileprotocol.ConflictRenameNew {
			destination = availableWindowsName(destination)
			components[len(components)-1] = filepath.Base(destination)
		}
	}
	identifier, err := randomMutationID()
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}
	temporary := destination + ".xenomorph-" + identifier
	output, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}
	written, copyErr := io.Copy(output, io.LimitReader(input, info.Size()+1))
	if copyErr == nil && written != info.Size() {
		copyErr = fsConflict("source changed during copy")
	}
	if copyErr == nil {
		copyErr = output.Sync()
	}
	if closeErr := output.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(temporary)
		return fileprotocol.ItemResult{}, copyErr
	}
	if conflict == fileprotocol.ConflictReplace {
		_ = os.Remove(destination)
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return fileprotocol.ItemResult{}, err
	}
	return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(components), BytesApplied: written}, nil
}

func availableWindowsName(path string) string {
	directory, name := filepath.Dir(path), filepath.Base(path)
	for suffix := 1; suffix <= 100; suffix++ {
		candidate := filepath.Join(directory, fmt.Sprintf("%s (%d)", name, suffix))
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
	return filepath.Join(directory, name+"-copy")
}

func verbNeedsSource(verb fileprotocol.MutationVerb) bool {
	return verb != fileprotocol.MutationCreateFile && verb != fileprotocol.MutationCreateDirectory
}
func verbNeedsDestination(verb fileprotocol.MutationVerb) bool {
	return verb == fileprotocol.MutationCreateFile || verb == fileprotocol.MutationCreateDirectory || verb == fileprotocol.MutationRename || verb == fileprotocol.MutationMove || verb == fileprotocol.MutationCopy || verb == fileprotocol.MutationDuplicate
}
func joinRelative(components []string) string { return strings.Join(components, "/") }

type conflictError struct{ message string }

func (err conflictError) Error() string        { return err.message }
func (err conflictError) Is(target error) bool { return target == os.ErrExist }
func fsConflict(message string) error          { return conflictError{message: message} }
func randomMutationID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
