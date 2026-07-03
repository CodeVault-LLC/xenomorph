package transport

import "testing"

func TestScreenSessionsTracksViewers(t *testing.T) {
	sessions := NewScreenSessions()

	if got, total := sessions.BeginViewer("agent-1"); got != 1 || total != 1 {
		t.Fatalf("expected first viewer counts 1/1, got %d/%d", got, total)
	}
	if got, total := sessions.BeginViewer("agent-1"); got != 2 || total != 2 {
		t.Fatalf("expected second viewer counts 2/2, got %d/%d", got, total)
	}
	if got, total := sessions.BeginViewer("agent-2"); got != 1 || total != 3 {
		t.Fatalf("expected other agent viewer counts 1/3, got %d/%d", got, total)
	}
	if got, total := sessions.EndViewer("agent-1"); got != 1 || total != 2 {
		t.Fatalf("expected remaining viewer counts 1/2, got %d/%d", got, total)
	}
	if got, total := sessions.EndViewer("agent-1"); got != 0 || total != 1 {
		t.Fatalf("expected final agent viewer counts 0/1, got %d/%d", got, total)
	}
	if got, total := sessions.EndViewer("agent-1"); got != 0 || total != 1 {
		t.Fatalf("expected repeated end counts 0/1, got %d/%d", got, total)
	}
}
