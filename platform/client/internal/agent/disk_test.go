package agent

import "testing"

func TestSameMountpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		left  string
		right string
		want  bool
	}{
		{name: "unix root", left: "/", right: "/", want: true},
		{name: "windows root", left: `C:\`, right: `c:`, want: true},
		{name: "different volume", left: `C:\`, right: `D:\`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sameMountpoint(tt.left, tt.right); got != tt.want {
				t.Fatalf("sameMountpoint(%q, %q) = %t, want %t", tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestHasMountOption(t *testing.T) {
	t.Parallel()
	if !hasMountOption([]string{"rw", "nosuid", "ro"}, "ro") {
		t.Fatal("hasMountOption() did not find read-only option")
	}
	if hasMountOption([]string{"rw", "nosuid"}, "ro") {
		t.Fatal("hasMountOption() reported absent read-only option")
	}
}
