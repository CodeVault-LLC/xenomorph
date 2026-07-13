package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFileRootProbeDoesNotRequireBrowserCredentials(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodPost, "/api/clients/agent-1/files/roots/probe", nil)
	response := httptest.NewRecorder()

	handleFileRootProbe(response, request, DashboardRuntime{})

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.Body.String(), "file workspace is not configured") {
		t.Fatalf("body = %q, want workspace configuration error", response.Body.String())
	}
}
