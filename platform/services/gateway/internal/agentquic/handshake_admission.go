package agentquic

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

type admissionContextKey struct{}

type handshakeAdmission struct {
	mu              sync.Mutex
	incomplete      chan struct{}
	prefixes        map[string]*prefixBucket
	ratePerSecond   float64
	burst           float64
	maximumPrefixes int
	now             func() time.Time
	metrics         *Metrics
}

type prefixBucket struct {
	tokens   float64
	updated  time.Time
	lastSeen time.Time
}

type admissionTicket struct {
	once      sync.Once
	admission *handshakeAdmission
}

func newHandshakeAdmission(config Config, metrics *Metrics) *handshakeAdmission {
	return &handshakeAdmission{
		incomplete:      make(chan struct{}, config.MaximumIncompleteHandshakes),
		prefixes:        make(map[string]*prefixBucket),
		ratePerSecond:   config.SourcePrefixRatePerSecond,
		burst:           float64(config.SourcePrefixBurst),
		maximumPrefixes: config.MaximumSourcePrefixes,
		now:             time.Now,
		metrics:         metrics,
	}
}

func (admission *handshakeAdmission) connectionContext(parent context.Context, remoteAddress net.Addr) (context.Context, error) {
	if admission == nil {
		return nil, fmt.Errorf("admit QUIC handshake: admission controller is nil")
	}

	select {
		case admission.incomplete <- struct{}{}:
		default:
			admission.metrics.overloadRejected.Add(1)
			return nil, fmt.Errorf("admit QUIC handshake: incomplete handshake capacity reached")
	}

	if !admission.allowPrefix(remoteAddress) {
		<-admission.incomplete
		admission.metrics.handshakeRejected.Add(1)

		return nil, fmt.Errorf("admit QUIC handshake: source-prefix rate exceeded")
	}

	ticket := &admissionTicket{admission: admission}
	context.AfterFunc(parent, ticket.release)

	return context.WithValue(parent, admissionContextKey{}, ticket), nil
}

func (admission *handshakeAdmission) allowPrefix(address net.Addr) bool {
	prefix := sourcePrefix(address)
	now := admission.now()
	admission.mu.Lock()
	defer admission.mu.Unlock()

	bucket, exists := admission.prefixes[prefix]
	if !exists {
		if len(admission.prefixes) >= admission.maximumPrefixes {
			admission.evictOldestPrefix()
		}

		if len(admission.prefixes) >= admission.maximumPrefixes {
			return false
		}

		bucket = &prefixBucket{tokens: admission.burst, updated: now}
		admission.prefixes[prefix] = bucket
	}

	elapsed := now.Sub(bucket.updated).Seconds()
	bucket.tokens = min(admission.burst, bucket.tokens+elapsed*admission.ratePerSecond)
	bucket.updated = now
	bucket.lastSeen = now

	if bucket.tokens < 1 {
		return false
	}

	bucket.tokens--

	return true
}

func (admission *handshakeAdmission) evictOldestPrefix() {
	var oldestKey string

	var oldest time.Time
	for key, bucket := range admission.prefixes {
		if oldestKey == "" || bucket.lastSeen.Before(oldest) {
			oldestKey = key
			oldest = bucket.lastSeen
		}
	}

	if oldestKey != "" {
		delete(admission.prefixes, oldestKey)
	}
}

func (ticket *admissionTicket) release() {
	if ticket == nil || ticket.admission == nil {
		return
	}

	ticket.once.Do(func() {
		<-ticket.admission.incomplete
	})
}

func ticketFromContext(ctx context.Context) *admissionTicket {
	ticket, _ := ctx.Value(admissionContextKey{}).(*admissionTicket)
	return ticket
}

func sourcePrefix(address net.Addr) string {
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		host = address.String()
	}

	ip := net.ParseIP(host)
	if ipv4 := ip.To4(); ipv4 != nil {
		return fmt.Sprintf("%d.%d.%d.0/24", ipv4[0], ipv4[1], ipv4[2])
	}

	if ipv6 := ip.To16(); ipv6 != nil {
		return fmt.Sprintf("%x:%x:%x:%x/56", ipv6[0:2], ipv6[2:4], ipv6[4:6], ipv6[6])
	}

	return "invalid"
}
