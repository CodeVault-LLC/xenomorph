package activity

import (
	"math"
	"testing"

	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
)

func TestClampTelemetryRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value float64
		want  float64
	}{
		{name: "negative", value: -1, want: 0},
		{name: "valid", value: 0.42, want: 0.42},
		{name: "above range", value: 2, want: 1},
		{name: "not a number", value: math.NaN(), want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := clampTelemetryRatio(tt.value); got != tt.want {
				t.Fatalf("clampTelemetryRatio(%f) = %f, want %f", tt.value, got, tt.want)
			}
		})
	}
}

func TestNormalizeApplicationTypesBoundsAndFiltersClientInput(t *testing.T) {
	t.Parallel()

	values := []*pb.ApplicationTypeUsage{
		{Category: "Development", Count: 150},
		{Category: "Untrusted category", Count: 100},
		{Category: "Browsers", Count: 100},
	}

	got := normalizeApplicationTypes(values)
	if len(got) != 2 {
		t.Fatalf("normalizeApplicationTypes() returned %d categories, want 2", len(got))
	}

	if got[0].Category != "Development" || got[0].Count != 150 {
		t.Fatalf("first normalized category = %#v", got[0])
	}

	if got[1].Category != "Browsers" || got[1].Count != 50 {
		t.Fatalf("second normalized category = %#v, want Browsers count 50", got[1])
	}
}

func TestNormalizeStorageBytesBoundsValues(t *testing.T) {
	t.Parallel()

	maximum := ^uint64(0)
	total, available, used := normalizeStorageBytes(maximum, maximum, maximum)

	const maxStorageBytes uint64 = 1 << 50

	if total != maxStorageBytes || available != maxStorageBytes || used != maxStorageBytes {
		t.Fatalf("normalizeStorageBytes() = %d, %d, %d", total, available, used)
	}
}
