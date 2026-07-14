// Package keyservice owns gateway cryptographic-provider validation, key
// generation, opaque private-key handles, key identifiers, lifecycle state,
// and fail-closed readiness. It does not authenticate agents, authorize
// operations, validate protobuf messages, or allocate distributed AEAD nonces.
// Provider identity and approved-mode status are server-authored observations;
// callers and clients cannot override them.
package keyservice
