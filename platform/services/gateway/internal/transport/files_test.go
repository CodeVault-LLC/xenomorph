package transport

import (
	"mime"
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

func TestDownloadContentDisposition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		relativePath string
		wantFilename string
	}{
		{
			name:         "hidden file",
			relativePath: "home/lukasolsen/.bash_logout",
			wantFilename: ".bash_logout",
		},
		{
			name:         "unicode filename",
			relativePath: "reports/årsrapport.pdf",
			wantFilename: "årsrapport.pdf",
		},
		{
			name:         "header control characters",
			relativePath: "reports/report\r\nmalicious.txt",
			wantFilename: "report__malicious.txt",
		},
		{
			name:         "missing filename",
			relativePath: "",
			wantFilename: "download",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			disposition := downloadContentDisposition(test.relativePath)
			mediaType, parameters, err := mime.ParseMediaType(disposition)
			if err != nil {
				t.Fatalf("parse content disposition %q: %v", disposition, err)
			}
			if mediaType != "attachment" {
				t.Errorf("media type = %q, want attachment", mediaType)
			}
			if parameters["filename"] != test.wantFilename {
				t.Errorf("filename = %q, want %q", parameters["filename"], test.wantFilename)
			}
			if strings.ContainsAny(disposition, "\r\n") {
				t.Errorf("content disposition contains a line break: %q", disposition)
			}
		})
	}
}
