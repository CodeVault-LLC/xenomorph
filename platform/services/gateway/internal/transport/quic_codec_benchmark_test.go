package transport

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

var benchmarkEncodedBytes []byte

func BenchmarkHeartbeatCodecs(b *testing.B) {
	for name, heartbeat := range heartbeatBenchmarkCorpus() {
		heartbeat := heartbeat
		protobufHeartbeat := heartbeatToProtobuf(heartbeat)
		b.Run(name+"/json", func(b *testing.B) {
			benchmarkCodec(b, func() ([]byte, error) { return json.Marshal(protobufHeartbeat) })
		})
		b.Run(name+"/deterministic-protobuf", func(b *testing.B) {
			benchmarkCodec(b, func() ([]byte, error) {
				return (proto.MarshalOptions{Deterministic: true}).Marshal(protobufHeartbeat)
			})
		})
		b.Run(name+"/xbp", func(b *testing.B) {
			benchmarkCodec(b, heartbeat.MarshalBinary)
		})
	}
}

func TestHeartbeatBenchmarkCorpusIsValid(t *testing.T) {
	t.Parallel()
	for name, heartbeat := range heartbeatBenchmarkCorpus() {
		if _, err := heartbeat.MarshalBinary(); err != nil {
			t.Errorf("%s XBP corpus value: %v", name, err)
		}
		if _, err := json.Marshal(heartbeatToProtobuf(heartbeat)); err != nil {
			t.Errorf("%s JSON corpus value: %v", name, err)
		}
		if _, err := (proto.MarshalOptions{Deterministic: true}).Marshal(heartbeatToProtobuf(heartbeat)); err != nil {
			t.Errorf("%s protobuf corpus value: %v", name, err)
		}
	}
}

func benchmarkCodec(b *testing.B, encode func() ([]byte, error)) {
	b.Helper()
	encoded, err := encode()
	if err != nil {
		b.Fatalf("encode benchmark value: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchmarkEncodedBytes, err = encode()
		if err != nil {
			b.Fatalf("encode benchmark value: %v", err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(encoded)), "encoded-bytes/op")
}

func heartbeatBenchmarkCorpus() map[string]wire.Heartbeat {
	minimum := wire.Heartbeat{}
	median := benchmarkHeartbeat(32, 3, 2)
	p95 := benchmarkHeartbeat(96, 12, 16)
	maximum := benchmarkHeartbeat(120, 25, 60)
	maximum.CPUModel = strings.Repeat("c", 160)
	maximum.StorageMountpoint = strings.Repeat("/", 4096)
	maximum.NetworkSSID = strings.Repeat("s", 128)
	return map[string]wire.Heartbeat{"minimum": minimum, "median": median, "p95": p95, "maximum-valid": maximum}
}

func benchmarkHeartbeat(textLength, gpuCount, addressCount int) wire.Heartbeat {
	text := strings.Repeat("x", textLength)
	gpus := make([]string, 0, gpuCount)
	for index := range gpuCount {
		gpus = append(gpus, fmt.Sprintf("gpu-%02d-%s", index, text))
	}
	addresses := make([]string, 0, addressCount)
	for index := range addressCount {
		addresses = append(addresses, fmt.Sprintf("2001:db8::%x", index))
	}
	return wire.Heartbeat{
		Presence: (uint64(1) << 29) - 1,
		Hostname: text, OSVersion: text, CPULoadPPM: 500_000, RAMUsagePPM: 500_000,
		UptimeSeconds: 1_000_000, CPUModel: text, CPUCores: 32, CPUThreads: 64,
		TotalRAMBytes: 64 << 30, GPUDevices: gpus, NetworkName: text,
		NetworkAddresses: addresses, KernelVersion: text, CPUFrequencyMHz: 5000,
		NetworkOnline: true, LinkSpeedMbps: 10_000, NetworkType: 1,
		TotalStorageBytes: 1 << 40, AvailableStorageBytes: 1 << 39, NetworkSSID: text,
		UsedStorageBytes: 1 << 39, StorageUsagePPM: 500_000, InodeUsagePPM: 100_000,
		StorageDevice: text, StorageFilesystem: text[:min(len(text), 64)], StorageMountpoint: text,
		StorageModel: text, StorageType: 1,
		ApplicationTypes: []wire.ApplicationUsage{{Category: 1, Count: 1}, {Category: 2, Count: 10}},
	}
}
