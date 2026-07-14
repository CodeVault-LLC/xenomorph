package transport

import "testing"

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
