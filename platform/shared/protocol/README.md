# XBP Protocol Source Contract

`xbp-v1.yaml` is the canonical reviewed source for XBP major version 1. It is
written in the JSON subset of YAML so the repository generator can parse it
with the Go standard library and does not add a runtime YAML dependency.

`registry-history.yaml` is append-only. Assigned IDs are never renamed or
reused. Its compatibility digest freezes each assigned revision, presence map,
field order, type, bound, and optional-bit assignment. Removed IDs move to
`tombstoned` and remain unavailable permanently.

Run `make wire-generate` at the repository root after changing either source.
The generator validates stream topology, IDs, field bounds, optional-bit
assignments, and history before producing Go codecs and the wire reference.
Generated files carry a generated-code header and must not be edited by hand.
It also emits an independently encoded structural-minimum frame and field
metadata for every registered message under `wire/testdata/golden/v1`. The wire
tests decode and canonically re-encode the complete corpus; the worked log-entry
vector additionally exercises semantic registry validation.
`go run ./cmd/wiregen -print-compatibility` from `platform/shared` prints the
candidate digests for explicit schema review; copying a digest is not approval
to change an existing wire layout.

Compatibility changes require a reviewed new message assignment/revision and
protocol minor migration rule, followed by an explicit compatibility-history
update. The generator rejects an unrecorded layout change.
Field reordering, required-field insertion into an existing revision, optional
bit reuse, ID reuse, and bound widening are incompatible changes and are not
permitted in place.
