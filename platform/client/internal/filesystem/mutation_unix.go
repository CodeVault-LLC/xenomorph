//go:build linux || darwin

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
	"strings"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
	"golang.org/x/sys/unix"
)

const (
	mutationFilePermission      = 0o600
	mutationDirectoryPermission = 0o700
	mutationIDBytes             = 16
)

func (root *rootHandle) mutate(verb fileprotocol.MutationVerb, conflict fileprotocol.ConflictStrategy, dryRun bool, item fileprotocol.MutationItem) (fileprotocol.ItemResult, error) {
	source, destination, err := root.validateMutation(verb, item)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	if dryRun {
		return root.planMutation(verb, source, destination)
	}

	return root.executeMutation(verb, conflict, item, source, destination)
}

func (root *rootHandle) validateMutation(verb fileprotocol.MutationVerb, item fileprotocol.MutationItem) ([]string, []string, error) {
	source, sourceErr := validateRelativePath(item.SourcePath)
	if verbNeedsSource(verb) {
		if sourceErr != nil {
			return nil, nil, sourceErr
		}

		if len(source) == 0 {
			return nil, nil, fmt.Errorf("filesystem root cannot be mutated")
		}

		if err := root.checkPreconditions(source, item.Preconditions); err != nil {
			return nil, nil, err
		}
	}

	destination, destinationErr := validateRelativePath(item.DestinationPath)
	if verbNeedsDestination(verb) {
		if destinationErr != nil {
			return nil, nil, destinationErr
		}

		if len(destination) == 0 {
			return nil, nil, fmt.Errorf("mutation destination is required")
		}
	}

	return source, destination, nil
}

func (root *rootHandle) planMutation(verb fileprotocol.MutationVerb, source, destination []string) (fileprotocol.ItemResult, error) {
	if verb == fileprotocol.MutationDelete {
		return root.planDelete(source)
	}

	if !verbNeedsDestination(verb) {
		return fileprotocol.ItemResult{State: "planned"}, nil
	}

	parent, _, err := root.openParent(destination)
	if parent != nil {
		_ = parent.Close()
	}

	return fileprotocol.ItemResult{State: "planned"}, err
}

func (root *rootHandle) planDelete(source []string) (fileprotocol.ItemResult, error) {
	if _, err := root.buildDeletePlan(source, maxRecursiveDeleteEntries); err != nil {
		return fileprotocol.ItemResult{}, err
	}

	return fileprotocol.ItemResult{State: "planned"}, nil
}

type deletePlanEntry struct {
	components []string
	device     uint64
	inode      uint64
	mode       uint32
}

func (root *rootHandle) buildDeletePlan(source []string, limit int) ([]deletePlanEntry, error) {
	parent, name, err := root.openParent(source)
	if err != nil {
		return nil, err
	}

	defer closeFileAfterRead(parent)

	parentFD, err := descriptorFromFile(parent)
	if err != nil {
		return nil, err
	}

	info, err := deleteRootInfo(parentFD, name)
	if err != nil {
		return nil, err
	}

	plan := make([]deletePlanEntry, 0, 1)
	err = scanDeleteTree(parentFD, name, source, uint64(info.Dev), 0, limit, &plan)

	if err == nil && (len(plan) == 0 || !sameDeleteIdentity(plan[0], info)) {
		err = fsConflict("entry changed during recursive deletion planning")
	}

	return plan, err
}

func deleteRootInfo(parentFD int, name string) (unix.Stat_t, error) {
	var parentInfo unix.Stat_t
	if err := unix.Fstat(parentFD, &parentInfo); err != nil {
		return unix.Stat_t{}, err
	}

	var info unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &info, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return unix.Stat_t{}, err
	}

	if info.Mode&unix.S_IFMT == unix.S_IFDIR && info.Dev != parentInfo.Dev {
		return unix.Stat_t{}, fmt.Errorf("recursive deletion cannot cross a filesystem boundary")
	}

	return info, nil
}

func sameDeleteIdentity(entry deletePlanEntry, info unix.Stat_t) bool {
	return entry.device == uint64(info.Dev) && entry.inode == uint64(info.Ino) && entry.mode&unix.S_IFMT == uint32(info.Mode)&unix.S_IFMT
}

func scanDeleteTree(parentFD int, name string, components []string, device uint64, depth, limit int, plan *[]deletePlanEntry) error {
	if len(*plan) >= limit {
		return fmt.Errorf("recursive deletion exceeds the %d-entry limit", limit)
	}

	var info unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &info, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}

	if uint64(info.Dev) != device {
		return fmt.Errorf("recursive deletion cannot cross a filesystem boundary")
	}

	entry := deletePlanEntry{
		components: append([]string(nil), components...),
		device:     uint64(info.Dev),
		inode:      uint64(info.Ino),
		mode:       uint32(info.Mode),
	}
	*plan = append(*plan, entry)

	if info.Mode&unix.S_IFMT != unix.S_IFDIR {
		return nil
	}

	return scanDeleteDirectory(parentFD, name, components, device, depth, limit, plan)
}

func scanDeleteDirectory(parentFD int, name string, components []string, device uint64, depth, limit int, plan *[]deletePlanEntry) error {
	if depth >= maxRecursiveDeleteDepth {
		return fmt.Errorf("recursive deletion exceeds the %d-level limit", maxRecursiveDeleteDepth)
	}

	directoryFD, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}

	directory, err := fileFromDescriptor(directoryFD, "recursive-delete-scan")
	if err != nil {
		_ = unix.Close(directoryFD)
		return err
	}

	defer closeFileAfterRead(directory)

	for {
		names, readErr := directory.Readdirnames(deleteScanBatchSize)
		for _, childName := range names {
			childComponents := append(append([]string(nil), components...), childName)
			if err := scanDeleteTree(directoryFD, childName, childComponents, device, depth+1, limit, plan); err != nil {
				return err
			}
		}

		if errors.Is(readErr, io.EOF) {
			return nil
		}

		if readErr != nil {
			return readErr
		}
	}
}

func (root *rootHandle) executeMutation(verb fileprotocol.MutationVerb, conflict fileprotocol.ConflictStrategy, item fileprotocol.MutationItem, source, destination []string) (fileprotocol.ItemResult, error) {
	switch verb {
	case fileprotocol.MutationCreateFile:
		return root.createFile(destination)
	case fileprotocol.MutationCreateDirectory:
		return root.createDirectory(destination)
	case fileprotocol.MutationTouch:
		return root.touch(source)
	case fileprotocol.MutationTruncate:
		return root.truncate(source, item.TruncateSize)
	case fileprotocol.MutationAppend:
		return root.appendFile(source, item.AppendData)
	default:
		return root.executeOrganizationMutation(verb, conflict, source, destination)
	}
}

func (root *rootHandle) executeOrganizationMutation(verb fileprotocol.MutationVerb, conflict fileprotocol.ConflictStrategy, source, destination []string) (fileprotocol.ItemResult, error) {
	switch verb {
	case fileprotocol.MutationRename, fileprotocol.MutationMove:
		return root.rename(source, destination, conflict)
	case fileprotocol.MutationCopy, fileprotocol.MutationDuplicate:
		return root.copyFile(source, destination, conflict)
	case fileprotocol.MutationDelete:
		return root.deleteEntry(source)
	default:
		return fileprotocol.ItemResult{}, fmt.Errorf("mutation is not supported")
	}
}

func (root *rootHandle) applyUpload(ctx context.Context, request fileprotocol.TransferRequest, plane TransferPlane) (fileprotocol.TransferResult, error) {
	components, err := validateRelativePath(request.Manifest.RelativePath)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}

	if hasPreconditions(request.Manifest.Preconditions) {
		if err := root.checkPreconditions(components, request.Manifest.Preconditions); err != nil {
			return fileprotocol.TransferResult{}, err
		}
	}

	stage, err := root.openUploadStage(components)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}

	defer closeFileAfterRead(stage.parent)

	published := false

	defer func() {
		if !published {
			_ = unix.Unlinkat(stage.parentFD, stage.temporaryName, 0)
		}
	}()

	acknowledged, err := receiveUploadChunks(ctx, request, plane, stage.file)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}

	skipped, err := publishUpload(stage, request.Manifest.Conflict)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}

	if skipped {
		return fileprotocol.TransferResult{ProtocolVersion: fileprotocol.Version, TransferID: request.Manifest.TransferID, State: "skipped", Scanning: "not_scanned"}, nil
	}

	published = true

	return fileprotocol.TransferResult{ProtocolVersion: fileprotocol.Version, TransferID: request.Manifest.TransferID, State: "completed", Acknowledged: acknowledged, BytesVerified: request.Manifest.Size, Scanning: "not_scanned"}, nil
}

type uploadStage struct {
	parent          *os.File
	parentFD        int
	destinationName string
	temporaryName   string
	file            *os.File
}

func (root *rootHandle) openUploadStage(components []string) (uploadStage, error) {
	parent, name, err := root.openParent(components)
	if err != nil {
		return uploadStage{}, err
	}

	parentFD, err := descriptorFromFile(parent)
	if err != nil {
		_ = parent.Close()
		return uploadStage{}, err
	}

	temporaryID, err := randomMutationID()
	if err != nil {
		_ = parent.Close()
		return uploadStage{}, err
	}

	temporaryName := ".xenomorph-upload-" + temporaryID

	fd, err := unix.Openat(parentFD, temporaryName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, mutationFilePermission)
	if err != nil {
		_ = parent.Close()
		return uploadStage{}, err
	}

	file, err := fileFromDescriptor(fd, "staged-upload")
	if err != nil {
		_ = unix.Close(fd)
		_ = parent.Close()

		return uploadStage{}, err
	}

	return uploadStage{parent: parent, parentFD: parentFD, destinationName: name, temporaryName: temporaryName, file: file}, nil
}

func receiveUploadChunks(ctx context.Context, request fileprotocol.TransferRequest, plane TransferPlane, file *os.File) ([]int, error) {
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
			return nil, err
		}

		if int64(len(data)) != chunk.Size || transferDigest(data) != chunk.SHA256 {
			_ = file.Close()
			return nil, fmt.Errorf("staged chunk integrity mismatch")
		}

		if _, err := file.Write(data); err != nil {
			_ = file.Close()
			return nil, err
		}

		if _, err := whole.Write(data); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("hash staged upload: %w", err)
		}

		acknowledged = append(acknowledged, chunk.Index)
	}

	if hex.EncodeToString(whole.Sum(nil)) != request.Manifest.SHA256 {
		_ = file.Close()
		return nil, fmt.Errorf("staged object integrity mismatch")
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, err
	}

	if err := file.Close(); err != nil {
		return nil, err
	}

	return acknowledged, nil
}

func publishUpload(stage uploadStage, conflict fileprotocol.ConflictStrategy) (bool, error) {
	name := stage.destinationName
	if conflict == fileprotocol.ConflictSkip && entryExists(stage.parentFD, name) {
		return true, nil
	}

	if conflict == fileprotocol.ConflictRenameNew {
		available, err := availableName(stage.parentFD, name)
		if err != nil {
			return false, err
		}

		name = available
	}

	return false, renameAt(stage.parentFD, stage.temporaryName, stage.parentFD, name, conflict == fileprotocol.ConflictReplace)
}

func (root *rootHandle) checkPreconditions(components []string, expected fileprotocol.Preconditions) error {
	info, err := root.statNoFollow(components)
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

	return validateObservedFile(info, expected)
}

func validateObservedFile(info os.FileInfo, expected fileprotocol.Preconditions) error {
	if expected.ExpectedKind != "" {
		if kindFromMode(info.Mode()) != expected.ExpectedKind {
			return fsConflict("target kind changed")
		}
	}

	if expected.ExpectedSize != nil {
		if info.Size() != *expected.ExpectedSize {
			return fsConflict("target size changed")
		}
	}

	if !expected.ExpectedModTime.IsZero() {
		if !info.ModTime().UTC().Equal(expected.ExpectedModTime.UTC()) {
			return fsConflict("target modification time changed")
		}
	}

	return nil
}

func (root *rootHandle) createFile(destination []string) (fileprotocol.ItemResult, error) {
	parent, name, err := root.openParent(destination)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	defer closeFileAfterRead(parent)

	fd, err := descriptorFromFile(parent)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	created, err := unix.Openat(fd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, mutationFilePermission)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	if err := unix.Close(created); err != nil {
		return fileprotocol.ItemResult{}, err
	}

	return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(destination)}, nil
}

func (root *rootHandle) createDirectory(destination []string) (fileprotocol.ItemResult, error) {
	parent, name, err := root.openParent(destination)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	defer closeFileAfterRead(parent)

	fd, err := descriptorFromFile(parent)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	if err := unix.Mkdirat(fd, name, mutationDirectoryPermission); err != nil {
		return fileprotocol.ItemResult{}, err
	}

	return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(destination)}, nil
}

func (root *rootHandle) touch(source []string) (fileprotocol.ItemResult, error) {
	parent, name, err := root.openParent(source)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	defer closeFileAfterRead(parent)

	fd, err := descriptorFromFile(parent)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	now := time.Now()

	times := []unix.Timespec{unix.NsecToTimespec(now.UnixNano()), unix.NsecToTimespec(now.UnixNano())}
	if err := unix.UtimesNanoAt(fd, name, times, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fileprotocol.ItemResult{}, err
	}

	return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(source)}, nil
}

func (root *rootHandle) truncate(source []string, size int64) (fileprotocol.ItemResult, error) {
	file, err := root.walk(source, unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	defer closeFileAfterRead(file)

	if err := file.Truncate(size); err != nil {
		return fileprotocol.ItemResult{}, err
	}

	return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(source), BytesApplied: size}, nil
}

func (root *rootHandle) appendFile(source []string, data []byte) (fileprotocol.ItemResult, error) {
	if len(data) > 1<<20 {
		return fileprotocol.ItemResult{}, fmt.Errorf("append data exceeds limit")
	}

	file, err := root.walk(source, unix.O_WRONLY|unix.O_APPEND|unix.O_CLOEXEC|unix.O_NOFOLLOW)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	defer closeFileAfterRead(file)

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

	return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(source), BytesApplied: int64(written)}, nil
}

func (root *rootHandle) rename(source, destination []string, conflict fileprotocol.ConflictStrategy) (fileprotocol.ItemResult, error) {
	sourceParent, sourceName, err := root.openParent(source)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	defer closeFileAfterRead(sourceParent)

	destinationParent, destinationName, err := root.openParent(destination)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	defer closeFileAfterRead(destinationParent)

	sourceFD, err := descriptorFromFile(sourceParent)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	destinationFD, err := descriptorFromFile(destinationParent)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	destinationName, skipped, err := resolveDestinationConflict(destinationFD, destinationName, conflict)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	if skipped {
		return fileprotocol.ItemResult{State: "skipped", ResultPath: joinRelative(destination)}, nil
	}

	destination[len(destination)-1] = destinationName

	if err := renameAt(sourceFD, sourceName, destinationFD, destinationName, conflict == fileprotocol.ConflictReplace); err != nil {
		if errors.Is(err, unix.EXDEV) {
			return root.moveAcrossDevice(source, destination, conflict, sourceFD, sourceName)
		}

		return fileprotocol.ItemResult{}, err
	}

	return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(destination)}, nil
}

func (root *rootHandle) moveAcrossDevice(source, destination []string, conflict fileprotocol.ConflictStrategy, sourceParentFD int, sourceName string) (fileprotocol.ItemResult, error) {
	result, err := root.copyFile(source, destination, conflict)
	if err != nil || result.State == "skipped" {
		return result, err
	}

	if err := unix.Unlinkat(sourceParentFD, sourceName, 0); err != nil {
		return fileprotocol.ItemResult{}, fmt.Errorf("remove verified cross-device source: %w", err)
	}

	return result, nil
}

func (root *rootHandle) copyFile(source, destination []string, conflict fileprotocol.ConflictStrategy) (fileprotocol.ItemResult, error) {
	sourceFile, info, err := root.openRegularFile(source)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	defer closeFileAfterRead(sourceFile)

	parent, name, err := root.openParent(destination)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	defer closeFileAfterRead(parent)

	parentFD, err := descriptorFromFile(parent)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	name, skipped, err := resolveDestinationConflict(parentFD, name, conflict)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	if skipped {
		return fileprotocol.ItemResult{State: "skipped", ResultPath: joinRelative(destination)}, nil
	}

	destination[len(destination)-1] = name

	temporaryName, written, err := stageCopy(parentFD, sourceFile, info)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	clean := false

	defer func() {
		if !clean {
			_ = unix.Unlinkat(parentFD, temporaryName, 0)
		}
	}()

	if err := renameAt(parentFD, temporaryName, parentFD, name, conflict == fileprotocol.ConflictReplace); err != nil {
		return fileprotocol.ItemResult{}, err
	}

	clean = true

	return fileprotocol.ItemResult{State: "completed", ResultPath: joinRelative(destination), BytesApplied: written}, nil
}

func resolveDestinationConflict(parentFD int, name string, conflict fileprotocol.ConflictStrategy) (string, bool, error) {
	if conflict == fileprotocol.ConflictSkip && entryExists(parentFD, name) {
		return name, true, nil
	}

	if conflict != fileprotocol.ConflictRenameNew {
		return name, false, nil
	}

	available, err := availableName(parentFD, name)

	return available, false, err
}

func stageCopy(parentFD int, source *os.File, info os.FileInfo) (string, int64, error) {
	identifier, err := randomMutationID()
	if err != nil {
		return "", 0, err
	}

	temporaryName := ".xenomorph-copy-" + identifier

	fd, err := unix.Openat(parentFD, temporaryName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(info.Mode().Perm()))
	if err != nil {
		return "", 0, err
	}

	temporary, err := fileFromDescriptor(fd, "staged-copy")
	if err != nil {
		_ = unix.Close(fd)
		return "", 0, err
	}

	written, err := io.Copy(temporary, io.LimitReader(source, info.Size()+1))
	if err == nil && written != info.Size() {
		err = fsConflict("source changed during copy")
	}

	if err == nil {
		err = temporary.Sync()
	}

	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		_ = unix.Unlinkat(parentFD, temporaryName, 0)
		return "", 0, err
	}

	return temporaryName, written, nil
}

func (root *rootHandle) deleteEntry(source []string) (fileprotocol.ItemResult, error) {
	plan, err := root.buildDeletePlan(source, maxRecursiveDeleteEntries)
	if err != nil {
		return fileprotocol.ItemResult{}, err
	}

	for index := len(plan) - 1; index >= 0; index-- {
		if err := root.deletePlannedEntry(plan[index]); err != nil {
			return fileprotocol.ItemResult{}, err
		}
	}

	return fileprotocol.ItemResult{State: "completed"}, nil
}

func (root *rootHandle) deletePlannedEntry(entry deletePlanEntry) error {
	parent, name, err := root.openParent(entry.components)
	if err != nil {
		return err
	}

	defer closeFileAfterRead(parent)

	parentFD, err := descriptorFromFile(parent)
	if err != nil {
		return err
	}

	var info unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &info, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}

	if !sameDeleteIdentity(entry, info) {
		return fsConflict("entry changed during recursive deletion")
	}

	flags := 0
	if info.Mode&unix.S_IFMT == unix.S_IFDIR {
		flags = unix.AT_REMOVEDIR
	}

	return unix.Unlinkat(parentFD, name, flags)
}

func (root *rootHandle) openParent(components []string) (*os.File, string, error) {
	if len(components) == 0 {
		return nil, "", fmt.Errorf("path has no parent")
	}

	parent, err := root.walk(components[:len(components)-1], unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW)

	return parent, components[len(components)-1], err
}

func entryExists(parentFD int, name string) bool {
	var stat unix.Stat_t
	return unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW) == nil
}

func availableName(parentFD int, name string) (string, error) {
	for suffix := 1; suffix <= 100; suffix++ {
		candidate := fmt.Sprintf("%s (%d)", name, suffix)
		if !entryExists(parentFD, candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no collision-free destination is available")
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
	value := make([]byte, mutationIDBytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate mutation identifier: %w", err)
	}

	return hex.EncodeToString(value), nil
}
