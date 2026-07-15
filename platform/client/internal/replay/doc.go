// Package replay owns the authenticated, bounded client command replay ledger.
// It persists only command identifiers, nonce digests, key/audience bindings,
// validity windows, and lifecycle state. It never stores command payloads,
// terminal output, paths, screenshots, logs, credentials, or result bodies.
//
// The ledger does not authenticate gateway commands or authorize execution.
// Those decisions remain with the command validator and verified gateway key.
package replay
