// Package wire implements the transport-independent Xenomorph Binary Protocol.
//
// The package owns canonical application framing, bounded primitive encodings,
// the append-only message registry, and connection-scoped replay detection. It
// does not own networking, TLS authentication, agent identity, authorization,
// persistence, or broker publication. Runtime identity must come from a
// gateway-authenticated session and never from values decoded by this package.
package wire
