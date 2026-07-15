// Package agentquic owns the client QUIC connection supervisor, XBP session
// negotiation, bounded lane writers/readers, acknowledgements, and reconnect
// policy. It does not assert agent identity, authorize operator intent, verify
// command signatures, execute commands, or assign gateway receipt truth.
package agentquic
