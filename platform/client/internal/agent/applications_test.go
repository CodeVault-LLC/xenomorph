package agent

import "testing"

func TestClassifyApplicationUsesAllowlistedCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "Mozilla Firefox", want: "Browsers"},
		{name: "Visual Studio Code", want: "Development"},
		{name: "Slack", want: "Communication"},
		{name: "VLC media player", want: "Media"},
		{name: "Steam", want: "Games"},
		{name: "LibreOffice Writer", want: "Productivity"},
		{name: "WireGuard VPN", want: "Security"},
		{name: "Calculator", want: "Utilities and other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := classifyApplication(tt.name); got != tt.want {
				t.Fatalf("classifyApplication(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestSummarizeApplicationTypesSortsByPrevalence(t *testing.T) {
	t.Parallel()

	applications := []string{"Firefox", "Chromium", "Calculator", "Clock", "Visual Studio Code"}

	got := summarizeApplicationTypes(applications)
	if len(got) != 3 {
		t.Fatalf("summarizeApplicationTypes() returned %d categories, want 3", len(got))
	}

	if got[0].Category != "Browsers" || got[0].Count != 2 {
		t.Fatalf("first category = %#v, want Browsers count 2", got[0])
	}

	if got[1].Category != "Utilities and other" || got[1].Count != 2 {
		t.Fatalf("second category = %#v, want Utilities and other count 2", got[1])
	}
}
