// Package agentquic owns the gateway UDP listener, QUIC/TLS admission,
// authenticated agent sessions, XBP stream topology, and connection fencing.
//
// It does not own agent identity derivation, authorization policy, durable
// operation state, broker publication, or the truth of decoded payloads. Agent
// identity is supplied only by the verified TLS peer certificate. Every XBP
// message body remains untrusted until the gateway ingress sink validates and
// commits it under a current session lease.
package agentquic
