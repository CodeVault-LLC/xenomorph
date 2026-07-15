package agentquic

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func TestTransferRegistryBindsSignedCapabilityScope(t *testing.T) {
	t.Parallel()
	registry := newTransferRegistry()
	request := fileprotocol.TransferRequest{
		ProtocolVersion: fileprotocol.Version,
		Manifest: fileprotocol.TransferManifest{
			ProtocolVersion: fileprotocol.Version,
			TransferID:      "5f9ee36a-80c2-4f32-9257-975b00236f98",
			Direction:       fileprotocol.TransferDownload,
			Size:            0,
			ChunkSize:       1,
		},
		Lease: fileprotocol.DataPlaneLease{Token: strings.Repeat("a", transferCapabilityHexLength), ExpiresAt: time.Now().Add(time.Hour)},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode transfer request: %v", err)
	}
	if err := registry.applySignedCommand(agent.CommandTypeFilesTransferPrepare, payload); err != nil {
		t.Fatalf("register signed transfer contract: %v", err)
	}
	if _, err := registry.contract(request.Manifest.TransferID, request.Lease.Token); err != nil {
		t.Fatalf("load matching transfer contract: %v", err)
	}
	if _, err := registry.contract(request.Manifest.TransferID, strings.Repeat("b", transferCapabilityHexLength)); err == nil {
		t.Fatal("wrong capability token was accepted")
	}
	if err := registry.applySignedCommand(agent.CommandTypeFilesTransferAbort, payload); err != nil {
		t.Fatalf("apply signed transfer abort: %v", err)
	}
	if _, err := registry.contract(request.Manifest.TransferID, request.Lease.Token); err == nil {
		t.Fatal("aborted transfer contract remained usable")
	}
}
