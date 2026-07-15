// Package clientbuild owns the fixed-toolchain construction of one temporary,
// compiled client profile. It does not authorize browser requests, assert
// client identity, or accept arbitrary compiler arguments.
package clientbuild

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxEndpointLength         = 253
	maxServerNameLength       = 253
	maxVersionLength          = 64
	maxArtifactBytes    int64 = 100 << 20
	buildTimeout              = 2 * time.Minute
	maxDiagnosticsBytes       = 16 << 10
	generatedFileMode         = 0o600
)

// ErrBusy indicates that the fixed client build capacity is already in use.
var ErrBusy = errors.New("client build capacity is unavailable")

// Request is browser-authored build intent. The gateway validates every field
// before using it to generate trusted source or select an allowlisted target.
type Request struct {
	Endpoint           string `json:"endpoint"`
	TLSServerName      string `json:"tls_server_name"`
	TargetOS           string `json:"target_os"`
	TargetArchitecture string `json:"target_architecture"`
	ClientVersion      string `json:"client_version"`
}

// Artifact is the generated client executable and its safe download filename.
type Artifact struct {
	Contents []byte
	Filename string
}

// Builder compiles the checked-in client and shared modules in a temporary
// workspace. It permits only one active build to bound resource use.
type Builder struct {
	sourceRoot string
	slots      chan struct{}
}

// New constructs a fixed-toolchain client builder from a trusted platform
// source tree containing the client and shared Go modules.
func New(sourceRoot string) (*Builder, error) {
	absSourceRoot, err := filepath.Abs(filepath.Clean(sourceRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve client build source: %w", err)
	}

	for _, module := range []string{"client", "shared"} {
		if _, err := os.Stat(filepath.Join(absSourceRoot, module, "go.mod")); err != nil {
			return nil, fmt.Errorf("validate client build source %s: %w", module, err)
		}
	}

	return &Builder{sourceRoot: absSourceRoot, slots: make(chan struct{}, 1)}, nil
}

// Validate normalizes and validates browser-authored build intent.
func (request *Request) Validate() error {
	request.Endpoint = strings.TrimSpace(request.Endpoint)
	request.TLSServerName = strings.TrimSpace(request.TLSServerName)
	request.TargetOS = strings.TrimSpace(request.TargetOS)
	request.TargetArchitecture = strings.TrimSpace(request.TargetArchitecture)
	request.ClientVersion = strings.TrimSpace(request.ClientVersion)

	if err := validateEndpoint(request.Endpoint); err != nil {
		return err
	}

	if !validDNSName(request.TLSServerName) || strings.EqualFold(request.TLSServerName, "localhost") {
		return fmt.Errorf("tls server name must be a non-localhost DNS name")
	}

	if !supportedTarget(request.TargetOS, request.TargetArchitecture) {
		return fmt.Errorf("target %s/%s is not supported", request.TargetOS, request.TargetArchitecture)
	}

	if !validVersion(request.ClientVersion) {
		return fmt.Errorf("client version must contain 1 to %d letters, numbers, dots, underscores, pluses, or hyphens", maxVersionLength)
	}

	return nil
}

// Build creates a temporary profile source file and compiles exactly one
// allowlisted client target. It never passes browser input as a compiler flag,
// path, environment variable, or shell fragment.
func (builder *Builder) Build(ctx context.Context, request Request) (Artifact, error) {
	if err := request.Validate(); err != nil {
		return Artifact{}, err
	}

	if !builder.acquire() {
		return Artifact{}, ErrBusy
	}
	defer builder.release()

	buildContext, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	workspace, err := os.MkdirTemp("", "xenomorph-client-build-")
	if err != nil {
		return Artifact{}, fmt.Errorf("create client build workspace: %w", err)
	}

	// The workspace is private temporary build state and is not an artifact store.
	defer func() { _ = os.RemoveAll(workspace) }()

	if err := builder.prepareWorkspace(workspace, request); err != nil {
		return Artifact{}, err
	}

	return compileArtifact(buildContext, workspace, request)
}

func (builder *Builder) acquire() bool {
	select {
	case builder.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (builder *Builder) release() {
	<-builder.slots
}

func (builder *Builder) prepareWorkspace(workspace string, request Request) error {
	if err := builder.copyModules(workspace); err != nil {
		return err
	}

	if err := writeGeneratedProfile(workspace, request); err != nil {
		return err
	}

	return nil
}

func compileArtifact(ctx context.Context, workspace string, request Request) (Artifact, error) {
	filename := artifactFilename(request.TargetOS, request.TargetArchitecture)
	outputPath := filepath.Join(workspace, filename)
	command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", outputPath, "./cmd") // #nosec G204 -- command and arguments are fixed; browser input cannot affect them.
	command.Dir = filepath.Join(workspace, "client")

	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+request.TargetOS, "GOARCH="+request.TargetArchitecture)

	diagnostics := limitedBuffer{limit: maxDiagnosticsBytes}
	command.Stderr = &diagnostics

	if err := command.Run(); err != nil {
		return Artifact{}, fmt.Errorf("build generated client: %w: %s", err, diagnostics.String())
	}

	contents, err := readArtifact(outputPath)
	if err != nil {
		return Artifact{}, err
	}

	return Artifact{Contents: contents, Filename: filename}, nil
}

func readArtifact(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect generated client artifact: %w", err)
	}

	if info.Size() <= 0 || info.Size() > maxArtifactBytes {
		return nil, fmt.Errorf("generated client artifact has invalid size")
	}

	contents, err := os.ReadFile(path) // #nosec G304 -- path is the builder's temporary output, not browser input.
	if err != nil {
		return nil, fmt.Errorf("read generated client artifact: %w", err)
	}

	return contents, nil
}

func (builder *Builder) copyModules(workspace string) error {
	for _, module := range []string{"client", "shared"} {
		destination := filepath.Join(workspace, module)
		if err := os.CopyFS(destination, os.DirFS(filepath.Join(builder.sourceRoot, module))); err != nil {
			return fmt.Errorf("copy %s module for client build: %w", module, err)
		}
	}

	return nil
}

func writeGeneratedProfile(workspace string, request Request) error {
	path := filepath.Join(workspace, "client", "internal", "config", "config_generated.go")
	contents := fmt.Sprintf(`//go:build !development

// Code generated by the gateway artifact builder. DO NOT EDIT.

package config

import "time"

func generatedConfig() Config {
	return Config{
		Environment: "production",
		ImplementationVersion: %q,
		TargetOS: %q,
		TargetArchitecture: %q,
		QUICEndpoint: %q,
		ServerName: %q,
		ClientCertificateFile: "client.crt",
		ClientPrivateKeyFile: "client.key",
		CAFile: "ca.crt",
		CommandVerificationKeyFile: "command-signing.pub",
		ReplayLedgerFile: "command-replay-ledger.json",
		ReplayAuthenticationKeyFile: "command-replay.key",
		HeartbeatInterval: 15 * time.Second,
		OperationTimeout: 10 * time.Second,
		QUICHandshakeTimeout: 5 * time.Second,
		QUICIdleTimeout: 45 * time.Second,
		QUICKeepAlive: 10 * time.Second,
		ReconnectMinimumBackoff: time.Second,
		ReconnectMaximumBackoff: 30 * time.Second,
	}
	}
	`, request.ClientVersion, request.TargetOS, request.TargetArchitecture, request.Endpoint, request.TLSServerName)

	if err := os.WriteFile(path, []byte(contents), generatedFileMode); err != nil {
		return fmt.Errorf("write generated client profile: %w", err)
	}

	return nil
}

func validateEndpoint(endpoint string) error {
	if len(endpoint) == 0 || len(endpoint) > maxEndpointLength {
		return fmt.Errorf("endpoint must contain 1 to %d bytes", maxEndpointLength)
	}

	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		return fmt.Errorf("endpoint must be a non-localhost host and port")
	}

	if err := validateEndpointHost(host); err != nil {
		return err
	}

	return validatePort(portText)
}

func validateEndpointHost(host string) error {
	if host == "" || strings.EqualFold(host, "localhost") {
		return fmt.Errorf("endpoint must be a non-localhost host and port")
	}

	if net.ParseIP(host) == nil && !validDNSName(host) {
		return fmt.Errorf("endpoint host must be an IP address or DNS name")
	}

	return nil
}

func validatePort(portText string) error {
	port, err := strconv.Atoi(portText)
	if err != nil {
		return fmt.Errorf("endpoint port must be in [1,65535]")
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("endpoint port must be in [1,65535]")
	}

	return nil
}

func validDNSName(name string) bool {
	if len(name) == 0 || len(name) > maxServerNameLength || net.ParseIP(name) != nil {
		return false
	}

	labels := strings.Split(name, ".")
	for _, label := range labels {
		if !validDNSLabel(label) {
			return false
		}
	}

	return true
}

func validDNSLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 {
		return false
	}

	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}

	for _, character := range label {
		if !validDNSCharacter(character) {
			return false
		}
	}

	return true
}

func validDNSCharacter(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' || character == '-'
}

func supportedTarget(targetOS, targetArchitecture string) bool {
	return (targetOS == "linux" || targetOS == "darwin" || targetOS == "windows") &&
		(targetArchitecture == "amd64" || targetArchitecture == "arm64")
}

func validVersion(version string) bool {
	if len(version) == 0 || len(version) > maxVersionLength {
		return false
	}

	for _, character := range version {
		if !validVersionCharacter(character) {
			return false
		}
	}

	return true
}

func validVersionCharacter(character rune) bool {
	return validDNSCharacter(character) || character == '.' || character == '_' || character == '+'
}

func artifactFilename(targetOS, targetArchitecture string) string {
	filename := "xenomorph-client-" + targetOS + "-" + targetArchitecture
	if targetOS == "windows" {
		return filename + ".exe"
	}

	return filename
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (buffer *limitedBuffer) Write(contents []byte) (int, error) {
	originalLength := len(contents)
	remaining := buffer.limit - buffer.Len()

	if remaining > 0 {
		if len(contents) > remaining {
			contents = contents[:remaining]
		}

		_, _ = buffer.Buffer.Write(contents)
	}

	return originalLength, nil
}
