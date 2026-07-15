package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/clientbuild"
)

type clientBuilderStub struct {
	artifact clientbuild.Artifact
	err      error
	request  clientbuild.Request
}

func (stub *clientBuilderStub) Build(_ context.Context, request clientbuild.Request) (clientbuild.Artifact, error) {
	stub.request = request
	return stub.artifact, stub.err
}

func TestClientBuildRouteDownloadsArtifact(t *testing.T) {
	t.Parallel()

	builder := &clientBuilderStub{artifact: clientbuild.Artifact{
		Contents: []byte("client-binary"), Filename: "xenomorph-client-linux-amd64",
	}}
	mux := http.NewServeMux()
	registerDashboardRoutes(mux, DashboardRuntime{ClientBuilder: builder})

	request := httptest.NewRequest(http.MethodPost, "/api/client-builds", strings.NewReader(`{"endpoint":"gateway.example.test:8444","tls_server_name":"gateway.example.test","target_os":"linux","target_architecture":"amd64","client_version":"1.2.3"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got, want := response.Header().Get("Content-Disposition"), "attachment; filename=xenomorph-client-linux-amd64"; got != want {
		t.Fatalf("Content-Disposition = %q, want %q", got, want)
	}
	if got, want := response.Body.String(), "client-binary"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if builder.request.Endpoint != "gateway.example.test:8444" {
		t.Fatalf("builder endpoint = %q", builder.request.Endpoint)
	}
}

func TestClientBuildRouteRejectsInvalidProfile(t *testing.T) {
	t.Parallel()

	builder := &clientBuilderStub{}
	mux := http.NewServeMux()
	registerDashboardRoutes(mux, DashboardRuntime{ClientBuilder: builder})

	request := httptest.NewRequest(http.MethodPost, "/api/client-builds", strings.NewReader(`{"endpoint":"gateway.example.test","tls_server_name":"gateway.example.test","target_os":"linux","target_architecture":"amd64","client_version":"1.2.3"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if builder.request.Endpoint != "" {
		t.Fatalf("builder received invalid request: %#v", builder.request)
	}
}

func TestClientBuildRouteReportsBusyCapacity(t *testing.T) {
	t.Parallel()

	builder := &clientBuilderStub{err: clientbuild.ErrBusy}
	mux := http.NewServeMux()
	registerDashboardRoutes(mux, DashboardRuntime{ClientBuilder: builder})

	request := httptest.NewRequest(http.MethodPost, "/api/client-builds", strings.NewReader(`{"endpoint":"gateway.example.test:8444","tls_server_name":"gateway.example.test","target_os":"linux","target_architecture":"amd64","client_version":"1.2.3"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
	if !errors.Is(builder.err, clientbuild.ErrBusy) {
		t.Fatal("test setup did not provide busy error")
	}
}
