package agentquic

import "sync/atomic"

// Metrics records bounded, low-cardinality QUIC admission and session counts.
type Metrics struct {
	handshakeRejected atomic.Uint64
	certificateFailed atomic.Uint64
	overloadRejected  atomic.Uint64
	activeSessions    atomic.Int64
	replacedSessions  atomic.Uint64
	fencedFrames      atomic.Uint64
	decodedFrames     atomic.Uint64
	rejectedFrames    atomic.Uint64
	qlogSessions      atomic.Uint64
	qlogBytes         atomic.Uint64
}

// MetricsSnapshot is a point-in-time copy of low-cardinality transport counters.
type MetricsSnapshot struct {
	HandshakeRejected uint64
	CertificateFailed uint64
	OverloadRejected  uint64
	ActiveSessions    int64
	ReplacedSessions  uint64
	FencedFrames      uint64
	DecodedFrames     uint64
	RejectedFrames    uint64
	QlogSessions      uint64
	QlogBytes         uint64
}

// Snapshot returns the current transport counters without resetting them.
func (metrics *Metrics) Snapshot() MetricsSnapshot {
	if metrics == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{
		HandshakeRejected: metrics.handshakeRejected.Load(),
		CertificateFailed: metrics.certificateFailed.Load(),
		OverloadRejected:  metrics.overloadRejected.Load(),
		ActiveSessions:    metrics.activeSessions.Load(),
		ReplacedSessions:  metrics.replacedSessions.Load(),
		FencedFrames:      metrics.fencedFrames.Load(),
		DecodedFrames:     metrics.decodedFrames.Load(),
		RejectedFrames:    metrics.rejectedFrames.Load(),
		QlogSessions:      metrics.qlogSessions.Load(),
		QlogBytes:         metrics.qlogBytes.Load(),
	}
}
