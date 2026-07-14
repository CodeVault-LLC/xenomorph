package transport

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/fileworkspace"
	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

const maxFileAPIRequestBytes int64 = 64 << 10

const maxFileChunkBytes int64 = 4 << 20

const (
	fileSearchMaxResults = 250
	fileSearchMaxEntries = 10_000
	fileSearchMaxDepth   = 16
)

type directoryListAPIRequest struct {
	RootID       string `json:"root_id"`
	RelativePath string `json:"relative_path"`
	Cursor       string `json:"cursor"`
	PageSize     int    `json:"page_size"`
}

type directorySearchAPIRequest struct {
	RootID       string `json:"root_id"`
	RelativePath string `json:"relative_path"`
	Query        string `json:"query"`
}

type metadataGetAPIRequest struct {
	RootID       string `json:"root_id"`
	RelativePath string `json:"relative_path"`
}

type metadataSetAPIRequest struct {
	RootID        string                     `json:"root_id"`
	RelativePath  string                     `json:"relative_path"`
	Preconditions fileprotocol.Preconditions `json:"preconditions"`
	Delta         fileprotocol.MetadataDelta `json:"delta"`
}

type archiveAPIRequest struct {
	RootID          string                        `json:"root_id"`
	Action          fileprotocol.ArchiveAction    `json:"action"`
	Format          fileprotocol.ArchiveFormat    `json:"format"`
	ArchivePath     string                        `json:"archive_path"`
	DestinationPath string                        `json:"destination_path"`
	SourcePaths     []string                      `json:"source_paths"`
	Conflict        fileprotocol.ConflictStrategy `json:"conflict_strategy"`
	Preconditions   fileprotocol.Preconditions    `json:"preconditions"`
}

type previewReadAPIRequest struct {
	RootID       string `json:"root_id"`
	RelativePath string `json:"relative_path"`
	Offset       int64  `json:"offset"`
	Length       int64  `json:"length"`
}

type transferCreateAPIRequest struct {
	Manifest fileprotocol.TransferManifest `json:"manifest"`
}

type mutationAPIRequest struct {
	RootID   string                        `json:"root_id"`
	Verb     fileprotocol.MutationVerb     `json:"verb"`
	DryRun   bool                          `json:"dry_run"`
	Conflict fileprotocol.ConflictStrategy `json:"conflict_strategy"`
	Items    []fileprotocol.MutationItem   `json:"items"`
}

type transferAPIResponse struct {
	Transfer fileworkspace.Transfer `json:"transfer"`
}
type transferListAPIResponse struct {
	Transfers []fileworkspace.Transfer `json:"transfers"`
}
type transferRemoveAPIResponse struct {
	Removed int `json:"removed"`
}

func registerFileRoutes(mux *http.ServeMux, runtime DashboardRuntime) {
	mux.HandleFunc("POST /api/clients/{agentID}/files/roots/probe", func(w http.ResponseWriter, request *http.Request) {
		handleFileRootProbe(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/directory", func(w http.ResponseWriter, request *http.Request) {
		handleDirectoryList(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/search", func(w http.ResponseWriter, request *http.Request) {
		handleDirectorySearch(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/metadata", func(w http.ResponseWriter, request *http.Request) {
		handleMetadataGet(w, request, runtime)
	})
	mux.HandleFunc("PATCH /api/clients/{agentID}/files/metadata", func(w http.ResponseWriter, request *http.Request) {
		handleMetadataSet(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/archives", func(w http.ResponseWriter, request *http.Request) {
		handleArchive(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/preview", func(w http.ResponseWriter, request *http.Request) {
		handlePreviewRead(w, request, runtime)
	})
	mux.HandleFunc("GET /api/clients/{agentID}/files/operations/{operationID}", func(w http.ResponseWriter, request *http.Request) {
		handleFileOperation(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/transfers", func(w http.ResponseWriter, request *http.Request) {
		handleTransferCreate(w, request, runtime)
	})
	mux.HandleFunc("GET /api/clients/{agentID}/files/transfers", func(w http.ResponseWriter, request *http.Request) {
		handleTransferList(w, request, runtime)
	})
	mux.HandleFunc("DELETE /api/clients/{agentID}/files/transfers", func(w http.ResponseWriter, request *http.Request) {
		handleFinishedTransferRemove(w, request, runtime)
	})
	mux.HandleFunc("GET /api/clients/{agentID}/files/transfers/{transferID}", func(w http.ResponseWriter, request *http.Request) {
		handleTransferGet(w, request, runtime)
	})
	mux.HandleFunc("DELETE /api/clients/{agentID}/files/transfers/{transferID}", func(w http.ResponseWriter, request *http.Request) {
		handleTransferRemove(w, request, runtime)
	})
	mux.HandleFunc("PUT /api/clients/{agentID}/files/transfers/{transferID}/chunks/{chunkIndex}", func(w http.ResponseWriter, request *http.Request) {
		handleTransferChunk(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/transfers/{transferID}/commit", func(w http.ResponseWriter, request *http.Request) {
		handleTransferCommit(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/transfers/{transferID}/resume", func(w http.ResponseWriter, request *http.Request) {
		handleTransferControl(w, request, runtime, true)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/transfers/{transferID}/abort", func(w http.ResponseWriter, request *http.Request) {
		handleTransferControl(w, request, runtime, false)
	})
	mux.HandleFunc("GET /api/clients/{agentID}/files/transfers/{transferID}/content", func(w http.ResponseWriter, request *http.Request) {
		handleTransferContent(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/mutations", func(w http.ResponseWriter, request *http.Request) {
		handleMutationCreate(w, request, runtime)
	})
}

func handleTransferList(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	writeDashboardJSON(w, http.StatusOK, transferListAPIResponse{Transfers: runtime.Files.Transfers(request.PathValue("agentID"))})
}

func handleTransferRemove(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	agentID := request.PathValue("agentID")
	transferID := request.PathValue("transferID")
	transfer, ok := runtime.Files.Transfer(agentID, transferID)
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "transfer not found"})
		return
	}
	if transfer.State != fileworkspace.TransferCompleted && transfer.State != fileworkspace.TransferFailed && transfer.State != fileworkspace.TransferCancelled {
		writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "active transfer must be cancelled before removal"})
		return
	}
	if err := runtime.Files.RemoveTransfer(agentID, runtime.FileOperatorID, request.Header.Get("X-Trace-ID"), transferID); err != nil {
		writeDashboardJSON(w, http.StatusInternalServerError, map[string]string{"error": "transfer could not be removed"})
		return
	}
	writeDashboardJSON(w, http.StatusOK, transferRemoveAPIResponse{Removed: 1})
}

func handleFinishedTransferRemove(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	removed, err := runtime.Files.RemoveFinishedTransfers(request.PathValue("agentID"), runtime.FileOperatorID, request.Header.Get("X-Trace-ID"))
	if err != nil {
		writeDashboardJSON(w, http.StatusInternalServerError, map[string]string{"error": "finished transfers could not be removed"})
		return
	}
	writeDashboardJSON(w, http.StatusOK, transferRemoveAPIResponse{Removed: removed})
}

func handleTransferControl(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime, resume bool) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	var (
		transfer fileworkspace.Transfer
		err      error
	)
	if resume {
		transfer, err = runtime.Files.ResumeTransfer(request.PathValue("agentID"), runtime.FileOperatorID, request.Header.Get("X-Trace-ID"), request.PathValue("transferID"))
	} else {
		transfer, err = runtime.Files.AbortTransfer(request.PathValue("agentID"), runtime.FileOperatorID, request.Header.Get("X-Trace-ID"), request.PathValue("transferID"))
	}
	if err != nil {
		writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "transfer state transition was rejected"})
		return
	}
	writeDashboardJSON(w, http.StatusAccepted, transferAPIResponse{Transfer: transfer})
}

func handleMutationCreate(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	var body mutationAPIRequest
	if !decodeFileAPIRequest(w, request, &body) {
		return
	}
	dispatchFileOperation(w, request, runtime, body.RootID, fileprotocol.CommandOperationExecute, &fileprotocol.MutationRequest{
		Verb: body.Verb, DryRun: body.DryRun, Conflict: body.Conflict, Items: body.Items,
	})
}

func handleTransferCreate(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	var body transferCreateAPIRequest
	if !decodeFileAPIRequest(w, request, &body) {
		return
	}
	agentID := request.PathValue("agentID")
	if _, ok := findClient(runtime.Directory, agentID); !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
		return
	}
	transfer, err := runtime.Files.CreateTransfer(agentID, runtime.FileOperatorID, request.Header.Get("X-Trace-ID"), body.Manifest)
	if err != nil {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "transfer manifest is invalid"})
		return
	}
	writeDashboardJSON(w, http.StatusAccepted, transferAPIResponse{Transfer: transfer})
}

func handleTransferGet(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	transfer, ok := runtime.Files.Transfer(request.PathValue("agentID"), request.PathValue("transferID"))
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "transfer not found"})
		return
	}
	writeDashboardJSON(w, http.StatusOK, transferAPIResponse{Transfer: transfer})
}

func handleTransferChunk(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	index, err := strconv.Atoi(request.PathValue("chunkIndex"))
	if err != nil || index < 0 {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "chunk index is invalid"})
		return
	}
	request.Body = http.MaxBytesReader(w, request.Body, maxFileChunkBytes)
	data, err := io.ReadAll(request.Body)
	if err != nil {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "transfer chunk is invalid"})
		return
	}
	transfer, err := runtime.Files.StageTransferChunk(request.PathValue("agentID"), request.PathValue("transferID"), index, data)
	if err != nil {
		writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "transfer chunk was rejected"})
		return
	}
	writeDashboardJSON(w, http.StatusOK, transferAPIResponse{Transfer: transfer})
}

func handleTransferCommit(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	transfer, err := runtime.Files.CommitUpload(request.PathValue("agentID"), runtime.FileOperatorID, request.Header.Get("X-Trace-ID"), request.PathValue("transferID"))
	if err != nil {
		writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "transfer could not be committed"})
		return
	}
	writeDashboardJSON(w, http.StatusAccepted, transferAPIResponse{Transfer: transfer})
}

func handleTransferContent(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	transfer, ok := runtime.Files.Transfer(request.PathValue("agentID"), request.PathValue("transferID"))
	if !ok || transfer.State != fileworkspace.TransferCompleted || transfer.Manifest.Direction != fileprotocol.TransferDownload {
		writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "download transfer is not complete"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", downloadContentDisposition(transfer.Manifest.RelativePath))
	w.Header().Set("Content-Length", strconv.FormatInt(transfer.Manifest.Size, 10))
	for index := range transfer.Manifest.Chunks {
		data, err := runtime.Files.ReadCompletedTransferChunk(transfer.AgentID, transfer.TransferID, index)
		if err != nil {
			return
		}
		if _, err := w.Write(data); err != nil {
			return
		}
	}
}

func downloadContentDisposition(relativePath string) string {
	filename := path.Base(relativePath)
	filename = strings.Map(func(character rune) rune {
		if character < ' ' || character == '\x7f' || character == '/' || character == '\\' {
			return '_'
		}
		return character
	}, filename)
	if filename == "." || filename == ".." || strings.TrimSpace(filename) == "" {
		filename = "download"
	}
	return mime.FormatMediaType("attachment", map[string]string{"filename": filename})
}

func handleFileRootProbe(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	agentID := request.PathValue("agentID")
	client, ok := findClient(runtime.Directory, agentID)
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
		return
	}
	if !client.IsOnline {
		writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "agent is offline"})
		return
	}
	operation, err := runtime.Files.ProbeRoots(agentID, runtime.FileOperatorID, request.Header.Get("X-Trace-ID"))
	if err != nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file root probe could not be dispatched"})
		return
	}
	writeDashboardJSON(w, http.StatusAccepted, map[string]any{"operation": operation})
}

func handleDirectoryList(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	var body directoryListAPIRequest
	if !decodeFileAPIRequest(w, request, &body) {
		return
	}
	dispatchFileOperation(w, request, runtime, body.RootID, fileprotocol.CommandDirectoryList, &fileprotocol.DirectoryListRequest{
		RelativePath: body.RelativePath, Cursor: body.Cursor, PageSize: body.PageSize,
	})
}

func handleDirectorySearch(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	var body directorySearchAPIRequest
	if !decodeFileAPIRequest(w, request, &body) {
		return
	}
	dispatchFileOperation(w, request, runtime, body.RootID, fileprotocol.CommandDirectorySearch, &fileprotocol.DirectorySearchRequest{
		RelativePath: body.RelativePath, Query: body.Query,
		MaxResults: fileSearchMaxResults, MaxEntries: fileSearchMaxEntries, MaxDepth: fileSearchMaxDepth,
	})
}

func handleMetadataGet(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	var body metadataGetAPIRequest
	if !decodeFileAPIRequest(w, request, &body) {
		return
	}
	dispatchFileOperation(w, request, runtime, body.RootID, fileprotocol.CommandMetadataGet, &fileprotocol.MetadataGetRequest{RelativePath: body.RelativePath})
}

func handleMetadataSet(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	var body metadataSetAPIRequest
	if !decodeFileAPIRequest(w, request, &body) {
		return
	}
	dispatchFileOperation(w, request, runtime, body.RootID, fileprotocol.CommandMetadataSet, &fileprotocol.MetadataSetRequest{
		RelativePath: body.RelativePath, Preconditions: body.Preconditions, Delta: body.Delta,
	})
}

func handleArchive(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	var body archiveAPIRequest
	if !decodeFileAPIRequest(w, request, &body) {
		return
	}
	dispatchFileOperation(w, request, runtime, body.RootID, fileprotocol.CommandArchiveExecute, &fileprotocol.ArchiveRequest{
		Action: body.Action, Format: body.Format, ArchivePath: body.ArchivePath,
		DestinationPath: body.DestinationPath, SourcePaths: body.SourcePaths,
		Conflict: body.Conflict, Preconditions: body.Preconditions,
	})
}

func handlePreviewRead(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	var body previewReadAPIRequest
	if !decodeFileAPIRequest(w, request, &body) {
		return
	}
	dispatchFileOperation(w, request, runtime, body.RootID, fileprotocol.CommandPreviewRead, &fileprotocol.PreviewReadRequest{
		RelativePath: body.RelativePath, Offset: body.Offset, Length: body.Length,
	})
}

func handleFileOperation(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	operation, ok := runtime.Files.Operation(request.PathValue("agentID"), request.PathValue("operationID"))
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "file operation not found"})
		return
	}
	writeDashboardJSON(w, http.StatusOK, map[string]any{"operation": operation})
}

func decodeFileAPIRequest(w http.ResponseWriter, request *http.Request, destination any) bool {
	request.Body = http.MaxBytesReader(w, request.Body, maxFileAPIRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file operation request"})
		return false
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file operation request"})
		return false
	}
	return true
}

func dispatchFileOperation(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime, rootID, commandType string, payload any) {
	if runtime.Files == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "file workspace is not configured"})
		return
	}
	agentID := request.PathValue("agentID")
	client, ok := findClient(runtime.Directory, agentID)
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
		return
	}
	if !client.IsOnline {
		writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "agent is offline"})
		return
	}
	operation, err := runtime.Files.Dispatch(agentID, runtime.FileOperatorID, strings.TrimSpace(rootID), commandType, request.Header.Get("X-Trace-ID"), payload)
	if err != nil {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "file operation request is invalid"})
		return
	}
	writeDashboardJSON(w, http.StatusAccepted, map[string]any{"operation": operation})
}
