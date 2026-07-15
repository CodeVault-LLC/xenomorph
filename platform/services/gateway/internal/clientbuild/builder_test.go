package clientbuild

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRequestValidate(t *testing.T) {
	t.Parallel()

	valid := Request{
		Endpoint: "gateway.example.test:8444", TLSServerName: "gateway.example.test",
		TargetOS: "linux", TargetArchitecture: "amd64", ClientVersion: "1.2.3",
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Request)
	}{
		{name: "missing port", mutate: func(request *Request) { request.Endpoint = "gateway.example.test" }},
		{name: "localhost endpoint", mutate: func(request *Request) { request.Endpoint = "localhost:8444" }},
		{name: "IP TLS name", mutate: func(request *Request) { request.TLSServerName = "192.0.2.10" }},
		{name: "unsupported target", mutate: func(request *Request) { request.TargetOS = "freebsd" }},
		{name: "invalid version", mutate: func(request *Request) { request.ClientVersion = "1.2.3;rm" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request := valid
			test.mutate(&request)

			if err := request.Validate(); err == nil {
				t.Fatal("Validate() succeeded for invalid request")
			}
		})
	}
}

func TestArtifactFilename(t *testing.T) {
	t.Parallel()

	if got, want := artifactFilename("windows", "amd64"), "xenomorph-client-windows-amd64.exe"; got != want {
		t.Fatalf("artifactFilename() = %q, want %q", got, want)
	}

	if got, want := artifactFilename("linux", "arm64"), "xenomorph-client-linux-arm64"; got != want {
		t.Fatalf("artifactFilename() = %q, want %q", got, want)
	}
}

func TestBuilderBuildsTemporaryProfile(t *testing.T) {
	t.Parallel()

	sourceRoot := t.TempDir()
	writeTestFile(t, filepath.Join(sourceRoot, "client", "go.mod"), "module example.test/client\n\ngo 1.25.12\n")
	writeTestFile(t, filepath.Join(sourceRoot, "client", "cmd", "main.go"), "package main\n\nimport \"example.test/client/internal/config\"\n\nfunc main() { _ = config.Config{} }\n")
	writeTestFile(t, filepath.Join(sourceRoot, "client", "internal", "config", "placeholder.go"), `package config

import "time"

type Config struct {
	Environment string
	ImplementationVersion string
	TargetOS string
	TargetArchitecture string
	QUICEndpoint string
	ServerName string
	ClientCertificateFile string
	ClientPrivateKeyFile string
	CAFile string
	CommandVerificationKeyFile string
	ReplayLedgerFile string
	ReplayAuthenticationKeyFile string
	HeartbeatInterval time.Duration
	OperationTimeout time.Duration
	QUICHandshakeTimeout time.Duration
	QUICIdleTimeout time.Duration
	QUICKeepAlive time.Duration
	ReconnectMinimumBackoff time.Duration
	ReconnectMaximumBackoff time.Duration
}
`)
	writeTestFile(t, filepath.Join(sourceRoot, "shared", "go.mod"), "module example.test/shared\n\ngo 1.25.12\n")

	builder, err := New(sourceRoot)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	artifact, err := builder.Build(context.Background(), Request{
		Endpoint: "gateway.example.test:8444", TLSServerName: "gateway.example.test",
		TargetOS: runtime.GOOS, TargetArchitecture: runtime.GOARCH, ClientVersion: "1.2.3",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(artifact.Contents) == 0 {
		t.Fatal("Build() returned an empty artifact")
	}
}

func TestWriteGeneratedProfileIncludesRequestValues(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "client", "internal", "config"), 0o700); err != nil {
		t.Fatalf("create config directory: %v", err)
	}

	request := Request{
		Endpoint: "gateway.example.test:8444", TLSServerName: "gateway.example.test",
		TargetOS: "linux", TargetArchitecture: "amd64", ClientVersion: "1.2.3",
	}
	if err := writeGeneratedProfile(workspace, request); err != nil {
		t.Fatalf("writeGeneratedProfile() error = %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(workspace, "client", "internal", "config", "config_generated.go")) // #nosec G304 -- test reads its own temporary workspace.

	if err != nil {
		t.Fatalf("read generated profile: %v", err)
	}

	for _, expected := range []string{request.Endpoint, request.TLSServerName, request.TargetOS, request.TargetArchitecture, request.ClientVersion} {
		if !strings.Contains(string(contents), expected) {
			t.Errorf("generated profile does not contain %q", expected)
		}
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
