package agentquic

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const (
	heartbeatOptionalFieldCount = 29
	partsPerMillion             = 1_000_000
	uuidVersionMask             = 0x0f
	uuidVersionFive             = 0x50
	uuidVariantMask             = 0x3f
	uuidRFC4122Variant          = 0x80
)

const (
	commandReasonPresence = uint64(1) << iota
	commandTerminalSessionPresence
	commandTerminalShellPresence
	commandTerminalDirectoryPresence
	commandTerminalExitPresence
	commandResultPresence
)

const (
	networkEthernet uint64 = iota + 1
	networkWireless
	networkLoopback
	networkOther
)

const (
	storageSolidState uint64 = iota + 1
	storageRotational
	storageFixed
	storageRemovable
	storageNetwork
	storageOptical
	storageOther
)

const (
	applicationBrowsers uint16 = iota + 1
	applicationDevelopment
	applicationCommunication
	applicationMedia
	applicationGames
	applicationProductivity
	applicationSecurity
	applicationOther
)

var logEventRegistry = map[string]wire.LogEvent{
	"runtime_started":                  wire.LogEventRuntimeStarted,
	"authentication_succeeded":         wire.LogEventAuthenticationSucceeded,
	"authentication_failed":            wire.LogEventAuthenticationFailed,
	"attestation_submitted":            wire.LogEventAttestationSubmitted,
	"attestation_failed":               wire.LogEventAttestationFailed,
	"heartbeat_failed":                 wire.LogEventHeartbeatFailed,
	"command_received":                 wire.LogEventCommandReceived,
	"command_completed":                wire.LogEventCommandCompleted,
	"command_poll_failed":              wire.LogEventCommandTransportFailed,
	"command_processing_failed":        wire.LogEventCommandProcessingFailed,
	"command_result_submission_failed": wire.LogEventCommandResultFailed,
	"runtime_loop_failed":              wire.LogEventRuntimeLoopFailed,
}

func heartbeatFromAgent(payload agent.HeartbeatPayload) wire.Heartbeat {
	applications := make([]wire.ApplicationUsage, 0, len(payload.ApplicationTypes))
	for _, usage := range payload.ApplicationTypes {
		applications = append(applications, wire.ApplicationUsage{
			Category: applicationCategory(usage.Category),
			Count:    usage.Count,
		})
	}

	return wire.Heartbeat{
		Presence: (uint64(1) << heartbeatOptionalFieldCount) - 1,
		Hostname: payload.Hostname, OSVersion: payload.OsVersion,
		CPULoadPPM: ratioPartsPerMillion(payload.CPULoad), RAMUsagePPM: ratioPartsPerMillion(payload.RAMUsage),
		UptimeSeconds: payload.UptimeSeconds, CPUModel: payload.CPUModel,
		CPUCores: nonnegativeInteger(payload.CPUCores), CPUThreads: nonnegativeInteger(payload.CPUThreads),
		TotalRAMBytes: payload.TotalRAMBytes, GPUDevices: payload.GPUDevices,
		NetworkName: payload.NetworkName, NetworkAddresses: payload.NetworkAddresses,
		KernelVersion: payload.KernelVersion, CPUFrequencyMHz: payload.CPUFrequencyMHz,
		NetworkOnline: payload.NetworkOnline, LinkSpeedMbps: payload.NetworkLinkSpeedMbps,
		NetworkType: networkType(payload.NetworkType), TotalStorageBytes: payload.TotalStorageBytes,
		AvailableStorageBytes: payload.AvailableStorageBytes, NetworkSSID: payload.NetworkSSID,
		UsedStorageBytes: payload.UsedStorageBytes, StorageUsagePPM: ratioPartsPerMillion(payload.StorageUsage),
		InodeUsagePPM: ratioPartsPerMillion(payload.StorageInodeUsage), StorageDevice: payload.StorageDevice,
		StorageFilesystem: payload.StorageFilesystem, StorageMountpoint: payload.StorageMountpoint,
		StorageModel: payload.StorageModel, StorageType: storageType(payload.StorageType),
		StorageReadOnly: payload.StorageReadOnly, ApplicationTypes: applications,
	}
}

func attestationFromAgent(payload agent.EndpointAttestation) wire.Attestation {
	browsers := make([]wire.BrowserObservation, 0, len(payload.Browsers))
	for _, browser := range payload.Browsers {
		browsers = append(browsers, wire.BrowserObservation{
			Name: browser.Name, BinaryPath: browser.BinaryPath, ProfileDirectory: browser.ProfileDir,
		})
	}

	return wire.Attestation{
		Hostname: payload.Hostname, OSVersion: payload.OSVersion,
		RequiresAttestation: payload.RequiresAttestation, Browsers: browsers,
		InstalledApplications: append([]string(nil), payload.InstalledApplications...),
	}
}

func logEntryFromAgent(payload agent.LogEntryPayload) (wire.LogEntry, error) {
	level, ok := logLevel(payload.Level)
	if !ok {
		return wire.LogEntry{}, fmt.Errorf("encode QUIC log entry: unregistered level %q", payload.Level)
	}

	component, ok := logComponent(payload.Component)
	if !ok {
		return wire.LogEntry{}, fmt.Errorf("encode QUIC log entry: unregistered component %q", payload.Component)
	}

	eventCode, detail, ok := logEvent(payload.Message)
	if !ok {
		return wire.LogEntry{}, fmt.Errorf("encode QUIC log entry: unregistered event metadata")
	}

	entry := wire.LogEntry{Level: uint64(level), Component: uint64(component), EventCode: uint64(eventCode), Detail: detail}
	if err := wire.ValidateLogEntry(entry); err != nil {
		return wire.LogEntry{}, err
	}

	return entry, nil
}

func commandResultFromAgent(payload agent.CommandResultPayload) (wire.CommandResult, error) {
	presence := uint64(0)
	if payload.Reason != "" {
		presence |= commandReasonPresence
	}

	if payload.TerminalSessionID != "" {
		presence |= commandTerminalSessionPresence
	}

	if payload.TerminalShell != "" {
		presence |= commandTerminalShellPresence
	}

	if payload.TerminalWorkingDirectory != "" {
		presence |= commandTerminalDirectoryPresence
	}

	if payload.TerminalExitCode != 0 {
		presence |= commandTerminalExitPresence
	}

	result := append([]byte(nil), payload.OutputData...)
	if len(result) == 0 {
		result = append(result, payload.Result...)
	}

	if len(result) > 0 {
		presence |= commandResultPresence
	}

	respondedAt, err := unixMilliseconds(payload.RespondedAt, "command response time")
	if err != nil {
		return wire.CommandResult{}, err
	}

	return wire.CommandResult{
		Presence: presence, CommandType: string(payload.Type), State: commandResultState(payload.Status),
		ReasonCode: 0, ReasonText: payload.Reason,
		RespondedAtMilliseconds: respondedAt, Hostname: payload.ClientHostname,
		TerminalSessionID: payload.TerminalSessionID, TerminalShell: payload.TerminalShell,
		TerminalWorkingDirectory: payload.TerminalWorkingDirectory,
		TerminalExitCode:         int64(payload.TerminalExitCode), ResultRevision: 1, Result: result,
	}, nil
}

func operationIDForPayload(domain, audience string, payload []byte) [16]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte("xenomorph/xbp/operation/" + domain + "\x00" + audience + "\x00"))
	_, _ = hash.Write(payload)
	digestBytes := hash.Sum(nil)

	var identifier [16]byte

	copy(identifier[:], digestBytes[:16])
	identifier[6] = (identifier[6] & uuidVersionMask) | uuidVersionFive
	identifier[8] = (identifier[8] & uuidVariantMask) | uuidRFC4122Variant

	return identifier
}

func parseOperationID(value string) ([16]byte, error) {
	identifier, err := uuid.Parse(value)
	if err != nil {
		return [16]byte{}, fmt.Errorf("parse operation ID %q: %w", value, err)
	}

	var result [16]byte

	copy(result[:], identifier[:])

	return result, nil
}

func ratioPartsPerMillion(value float64) uint64 {
	if math.IsNaN(value) || value <= 0 {
		return 0
	}

	if value >= 1 {
		return partsPerMillion
	}

	return uint64(math.Round(value * partsPerMillion))
}

func nonnegativeInteger(value int32) uint64 {
	if value < 0 {
		return 0
	}

	return uint64(value)
}

func networkType(value string) uint64 {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ethernet":
		return networkEthernet
	case "wireless", "wifi", "wi-fi":
		return networkWireless
	case "loopback":
		return networkLoopback
	case "":
		return 0
	default:
		return networkOther
	}
}

func storageType(value string) uint64 {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "solid-state", "ssd":
		return storageSolidState
	case "rotational", "hdd":
		return storageRotational
	case "fixed":
		return storageFixed
	case "removable":
		return storageRemovable
	case "network":
		return storageNetwork
	case "optical":
		return storageOptical
	case "unknown", "":
		return 0
	default:
		return storageOther
	}
}

func applicationCategory(value string) uint16 {
	switch value {
	case "Browsers":
		return applicationBrowsers
	case "Development":
		return applicationDevelopment
	case "Communication":
		return applicationCommunication
	case "Media":
		return applicationMedia
	case "Games":
		return applicationGames
	case "Productivity":
		return applicationProductivity
	case "Security":
		return applicationSecurity
	default:
		return applicationOther
	}
}

func commandResultState(status agent.CommandStatus) uint64 {
	if status == agent.CommandStatusExecuted {
		return uint64(wire.CommandResultStateExecuted)
	}

	return uint64(wire.CommandResultStateRejected)
}

func logLevel(value string) (wire.LogLevel, bool) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "DEBUG":
		return wire.LogLevelDebug, true
	case "INFO":
		return wire.LogLevelInfo, true
	case "WARN":
		return wire.LogLevelWarn, true
	case "ERROR":
		return wire.LogLevelError, true
	default:
		return 0, false
	}
}

func logComponent(value string) (wire.LogComponent, bool) {
	switch strings.TrimSpace(value) {
	case "client.runtime":
		return wire.LogComponentRuntime, true
	case "client.authentication":
		return wire.LogComponentAuthentication, true
	case "client.attestation":
		return wire.LogComponentAttestation, true
	case "client.heartbeat":
		return wire.LogComponentHeartbeat, true
	case "client.command":
		return wire.LogComponentCommand, true
	default:
		return 0, false
	}
}

func logEvent(value string) (wire.LogEvent, string, bool) {
	metadata := strings.TrimSpace(value)
	eventField, detail, found := strings.Cut(metadata, " detail=")

	if !found {
		eventField = metadata
		detail = ""
	}

	eventName, ok := strings.CutPrefix(eventField, "event=")
	if !ok {
		return 0, "", false
	}

	event, found := logEventRegistry[eventName]

	return event, detail, found
}
