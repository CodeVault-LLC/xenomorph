# QUIC Agent Transport Evidence and Compatibility Record

Status date: 2026-07-15.

## Current implementation evidence

| Area | Repository evidence | Decision state |
| --- | --- | --- |
| Protocol | Canonical schema/history, deterministic generator, generated reference, bounded primitives/frames, semantic validation, replay window, independently encoded golden frame/metadata for every message, negative tests, and fuzz targets. | Implemented; XBP production selection remains conditional. |
| Gateway | QUIC v1/TLS 1.3 mTLS listener, admission limits, certificate identity, negotiation, replacement fence, lanes, drain, acknowledgements, metrics, qlog gate, durable journals, broker translation, transfer/media adapters. | Disabled by default. |
| Client | Immutable configuration, strict TLS dialer, supervisor/backoff, control/event/command lanes, explicit fallback, replay ledger, transfer/media adapters, fixed logs. | Selected only by explicit client mode. |
| Trust tests | Real UDP integration covers successful mTLS/heartbeat commit, missing/untrusted/expired/not-yet-valid client credentials, wrong name, wrong ALPN, unsupported QUIC version, unsupported application version, and no unauthenticated ingress. | Local automated evidence. |
| Recovery tests | Command, operation, replay-ledger, and transfer tests cover duplicate/conflict and crash recovery states. | Local automated evidence; full process/OS chaos matrix pending. |
| Quality gates | The complete `make ci-go`, `gosec`, `go vet`, `staticcheck`, deterministic generation, race tests, and cross-builds pass locally on 2026-07-15. | Repeat the complete gate from the committed reviewed revision. |

## Codec measurement

`BenchmarkHeartbeatCodecs` compares the same logical minimum, median, p95, and
maximum-valid heartbeat using current JSON, deterministic protobuf, and XBP.
It reports encoded bytes, time, and allocations. Run it with:

```text
cd platform/services/gateway
go test ./internal/transport -run '^$' -bench BenchmarkHeartbeatCodecs -benchmem -count=5
```

A short local developer run on 2026-07-15 used Go 1.25.12, linux/amd64, and an
AMD Ryzen 7 5800X3D. It is a smoke measurement, not controlled-runner approval.
For median heartbeat the observed encoded sizes were JSON 1194 bytes,
deterministic protobuf 630 bytes, and XBP 535 bytes. For p95 they were 3144,
2557, and 2439 bytes. XBP did not demonstrate the proposed 20 percent size
advantage over protobuf for those samples and allocated more in the current
prototype. Therefore this result does not approve XBP for production. The full
family corpus, decode/validation/translation cost, packet-loss network tests,
controlled repetitions, and predeclared product threshold remain required.

## Dependency and cryptographic review

Both runtimes currently pin `github.com/quic-go/quic-go v0.59.1`; the repository
Go directive is 1.25.12. `make ci-go` runs module verification and
`govulncheck`. License, upstream release policy, supported-platform behavior,
exact transitive dependency inventory, and the interaction between `quic-go`,
Go TLS, the configured cryptographic provider, and any claimed validated FIPS
boundary require independent review. No FIPS or certification claim is created
by the QUIC implementation.

## Supported compatibility matrix

| Gateway | Client mode | Agent plane | Supported now |
| --- | --- | --- | --- |
| Dual-stack gateway, QUIC off | `http` | Existing HTTPS/WebSocket/HTTP transfer | Yes, compatibility baseline. |
| Dual-stack gateway, QUIC on in controlled environment | `quic` | QUIC/XBP required | Implemented, not production-approved. |
| Dual-stack gateway, QUIC on | `quic-first` with future expiry | QUIC/XBP; ordinary network failure may use HTTPS | Implemented rollout mode, not fleet policy. |
| QUIC-only gateway | any | Old routes removed | No; requires Phase 8 removal release. |
| Multiple active gateway instances | `quic` | Shared session ownership | No; distributed fencing is not implemented. |

XBP major 1 supports minor 0 only. The current and previous approved protocol
support window cannot be declared until a second approved version exists.
Command signature version remains the existing JSON canonical envelope and is
independent of XBP versioning.

## Capacity and release evidence still required

The approved reference profile must name CPU, memory, kernel, NIC, OS, Go,
`quic-go`, NATS, storage, load balancer/firewall, NAT, MTU, and network
conditions. Test idle connections, active heartbeat, command bursts, transfers,
media, reconnect storms, and adversarial traffic. Record p50/p95/p99 latency,
messages/useful bytes per second, CPU, allocations, heap, goroutines, UDP drops,
retransmission, broker/disk latency, queue occupancy, rejection, and recovery.

Required external or environment-dependent evidence remains:

- controlled loss/reordering/jitter/MTU/blackhole/NAT-rebinding tests;
- slow peer, stream flood, flow-control, disk-full, broker degradation, stateless
  reset, sleep/wake, and gateway restart exercises;
- soak beyond certificate, session, and NAT idle boundaries;
- native Linux, macOS, and Windows agent integration;
- certificate enrollment, renewal, revocation, and active-session fencing;
- mixed-version canary, rollback, zero-old-route observation, and removal proof;
- UDP kernel buffer/drop evidence on the selected gateway OS;
- architecture, gateway security, operations, dependency, cryptographic,
  protocol, and product approvals; and
- closure of all repository Milestone 1 Blocker and High findings.

No production listener, QUIC-only cohort, XBP selection, binary command
signature, datagram experiment, multi-instance gateway, or old-route removal is
approved until its applicable evidence is recorded and reviewed.
