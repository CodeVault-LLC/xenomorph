package transport

import (
	"testing"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/agentquic"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

func TestScreenSessionsTracksViewers(t *testing.T) {
	sessions := NewScreenSessions()
	tests := []struct {
		name        string
		update      func(string) (int, int)
		agentID     string
		wantAgent   int
		wantOverall int
	}{
		{name: "first viewer", update: sessions.BeginViewer, agentID: "agent-1", wantAgent: 1, wantOverall: 1},
		{name: "second viewer", update: sessions.BeginViewer, agentID: "agent-1", wantAgent: 2, wantOverall: 2},
		{name: "other agent", update: sessions.BeginViewer, agentID: "agent-2", wantAgent: 1, wantOverall: 3},
		{name: "remaining viewer", update: sessions.EndViewer, agentID: "agent-1", wantAgent: 1, wantOverall: 2},
		{name: "final agent viewer", update: sessions.EndViewer, agentID: "agent-1", wantAgent: 0, wantOverall: 1},
		{name: "repeated end", update: sessions.EndViewer, agentID: "agent-1", wantAgent: 0, wantOverall: 1},
	}
	for _, test := range tests {
		gotAgent, gotOverall := test.update(test.agentID)
		if gotAgent != test.wantAgent || gotOverall != test.wantOverall {
			t.Errorf("%s counts = %d/%d, want %d/%d", test.name, gotAgent, gotOverall, test.wantAgent, test.wantOverall)
		}
	}
}

func TestScreenSessionsAuthorizesExactMediaGeneration(t *testing.T) {
	sessions := NewScreenSessions()
	authorization := MediaGenerationAuthorization{
		GenerationID:      [16]byte{1},
		FrameRateCap:      60,
		MaximumFrameBytes: 10 << 20,
	}

	if sessions.AuthorizeMediaGeneration("agent-1", authorization) {
		t.Fatal("AuthorizeMediaGeneration without viewer = true, want false")
	}
	sessions.BeginViewer("agent-1")
	if !sessions.AuthorizeMediaGeneration("agent-1", authorization) {
		t.Fatal("AuthorizeMediaGeneration with viewer = false, want true")
	}

	tests := []struct {
		name              string
		agentID           string
		generationID      [16]byte
		frameRateCap      uint64
		maximumFrameBytes uint64
		want              bool
	}{
		{name: "exact contract", agentID: "agent-1", generationID: [16]byte{1}, frameRateCap: 60, maximumFrameBytes: 10 << 20, want: true},
		{name: "wrong agent", agentID: "agent-2", generationID: [16]byte{1}, frameRateCap: 60, maximumFrameBytes: 10 << 20},
		{name: "wrong generation", agentID: "agent-1", generationID: [16]byte{2}, frameRateCap: 60, maximumFrameBytes: 10 << 20},
		{name: "wrong frame rate", agentID: "agent-1", generationID: [16]byte{1}, frameRateCap: 30, maximumFrameBytes: 10 << 20},
		{name: "wrong frame limit", agentID: "agent-1", generationID: [16]byte{1}, frameRateCap: 60, maximumFrameBytes: 1024},
	}
	for _, test := range tests {
		if got := sessions.AuthorizesMediaOpen(test.agentID, test.generationID, test.frameRateCap, test.maximumFrameBytes); got != test.want {
			t.Errorf("%s authorization = %t, want %t", test.name, got, test.want)
		}
	}
	if !sessions.AuthorizesMediaFrame("agent-1", authorization.GenerationID) {
		t.Fatal("AuthorizesMediaFrame for active generation = false, want true")
	}
	if sessions.AuthorizesMediaFrame("agent-1", [16]byte{2}) {
		t.Fatal("AuthorizesMediaFrame for wrong generation = true, want false")
	}
}

func TestScreenSessionsRevokesMediaGeneration(t *testing.T) {
	sessions := NewScreenSessions()
	authorization := MediaGenerationAuthorization{
		GenerationID:      [16]byte{1},
		FrameRateCap:      60,
		MaximumFrameBytes: 10 << 20,
	}
	sessions.BeginViewer("agent-1")
	if !sessions.AuthorizeMediaGeneration("agent-1", authorization) {
		t.Fatal("AuthorizeMediaGeneration = false, want true")
	}

	sessions.RevokeMediaGeneration("agent-1")
	if sessions.AuthorizesMediaFrame("agent-1", authorization.GenerationID) {
		t.Fatal("authorization remained after explicit revocation")
	}
	if !sessions.AuthorizeMediaGeneration("agent-1", authorization) {
		t.Fatal("AuthorizeMediaGeneration after revocation = false, want true")
	}
	sessions.EndViewer("agent-1")
	if sessions.AuthorizesMediaFrame("agent-1", authorization.GenerationID) {
		t.Fatal("authorization remained after final viewer disconnected")
	}
}

func TestQUICMediaIngressRequiresGatewayAuthorizedGeneration(t *testing.T) {
	sessions := NewScreenSessions()
	sessions.BeginViewer("agent-1")
	authorization := MediaGenerationAuthorization{
		GenerationID:      [16]byte{1},
		FrameRateCap:      60,
		MaximumFrameBytes: 10 << 20,
	}
	if !sessions.AuthorizeMediaGeneration("agent-1", authorization) {
		t.Fatal("AuthorizeMediaGeneration = false, want true")
	}
	server := &Server{screenSessions: sessions, screenStore: NewScreenStore()}
	receipt := agentquic.IngressReceipt{AgentID: "agent-1"}

	validOpen := &wire.MediaOpen{
		GenerationID: authorization.GenerationID, Codec: 1, Width: 1920, Height: 1080,
		FrameRateCap: authorization.FrameRateCap, MaximumFrameBytes: authorization.MaximumFrameBytes,
	}
	if _, err := server.commitQUICMediaOpen(receipt, validOpen); err != nil {
		t.Fatalf("commitQUICMediaOpen(valid) error = %v", err)
	}
	wrongLimit := *validOpen
	wrongLimit.MaximumFrameBytes--
	if _, err := server.commitQUICMediaOpen(receipt, &wrongLimit); err == nil {
		t.Fatal("commitQUICMediaOpen(wrong limit) error = nil, want authorization failure")
	}

	validFrame := &wire.MediaFrame{
		GenerationID: authorization.GenerationID, FrameNumber: 1, ContentType: 1, Data: []byte("jpeg"),
	}
	if _, err := server.commitQUICMediaFrame(receipt, validFrame); err != nil {
		t.Fatalf("commitQUICMediaFrame(valid) error = %v", err)
	}
	frame, exists := server.screenStore.Latest("agent-1")
	if !exists || string(frame.Content) != "jpeg" {
		t.Fatalf("Latest frame = %q/%t, want committed JPEG", frame.Content, exists)
	}
	wrongGeneration := *validFrame
	wrongGeneration.GenerationID = [16]byte{2}
	if _, err := server.commitQUICMediaFrame(receipt, &wrongGeneration); err == nil {
		t.Fatal("commitQUICMediaFrame(wrong generation) error = nil, want authorization failure")
	}
}
