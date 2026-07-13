package transport

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

const maxFileAPIRequestBytes int64 = 16 << 10

type directoryListAPIRequest struct {
	RootID       string `json:"root_id"`
	RelativePath string `json:"relative_path"`
	Cursor       string `json:"cursor"`
	PageSize     int    `json:"page_size"`
}

type metadataGetAPIRequest struct {
	RootID       string `json:"root_id"`
	RelativePath string `json:"relative_path"`
}

type previewReadAPIRequest struct {
	RootID       string `json:"root_id"`
	RelativePath string `json:"relative_path"`
	Offset       int64  `json:"offset"`
	Length       int64  `json:"length"`
}

func registerFileRoutes(mux *http.ServeMux, runtime DashboardRuntime) {
	mux.HandleFunc("POST /api/clients/{agentID}/files/roots/probe", func(w http.ResponseWriter, request *http.Request) {
		handleFileRootProbe(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/directory", func(w http.ResponseWriter, request *http.Request) {
		handleDirectoryList(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/metadata", func(w http.ResponseWriter, request *http.Request) {
		handleMetadataGet(w, request, runtime)
	})
	mux.HandleFunc("POST /api/clients/{agentID}/files/preview", func(w http.ResponseWriter, request *http.Request) {
		handlePreviewRead(w, request, runtime)
	})
	mux.HandleFunc("GET /api/clients/{agentID}/files/operations/{operationID}", func(w http.ResponseWriter, request *http.Request) {
		handleFileOperation(w, request, runtime)
	})
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

func handleMetadataGet(w http.ResponseWriter, request *http.Request, runtime DashboardRuntime) {
	var body metadataGetAPIRequest
	if !decodeFileAPIRequest(w, request, &body) {
		return
	}
	dispatchFileOperation(w, request, runtime, body.RootID, fileprotocol.CommandMetadataGet, &fileprotocol.MetadataGetRequest{RelativePath: body.RelativePath})
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
