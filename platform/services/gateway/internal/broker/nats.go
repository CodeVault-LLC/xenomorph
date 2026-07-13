// Package broker implements the NATS JetStream message broker client for the
// gateway. This package owns the NATS connection lifecycle, JetStream context
// initialization, stream provisioning, and publish/subscribe operations.
package broker

import (
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

const (
	// systemEventsStream is the JetStream name for gateway ingress events.
	systemEventsStream string = "SYSTEM_EVENTS"
	// systemEventsSubject is the subject namespace captured by the stream.
	systemEventsSubject string = "sys.in.>"
)

// NATS wraps the NATS connection and JetStream context. All event publishing
// and subscription for the gateway flows through this type.
type NATS struct {
	Conn *nats.Conn
	JS   nats.JetStreamContext
}

// New connects to the NATS server at url and initializes the JetStream
// context. The SYSTEM_EVENTS stream is created if it does not exist.
//
// The stream configuration:
//   - Name: systemEventsStream ("SYSTEM_EVENTS").
//   - Subjects: systemEventsSubject ("sys.in.>"), the gateway ingress namespace prefix.
//   - Storage: FileStorage (persistent across NATS restarts)
//
// The stream is required for all gateway event publishing. If the stream
// already exists, New uses it as-is. Stream configuration changes are not
// applied automatically and must be managed through the NATS CLI or API.
func New(url string) (*NATS, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("NATS connection to %q failed: %w", url, err)
	}

	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("JetStream context initialization failed: %w", err)
	}

	if err := ensureSystemEventsStream(js); err != nil {
		return nil, err
	}

	return &NATS{Conn: nc, JS: js}, nil
}

// Close shuts down the NATS connection. Safe to call multiple times.
func (n *NATS) Close() {
	if n.Conn != nil {
		n.Conn.Close()
	}
}

// Publish marshals msg as protobuf and publishes it to the given subject
// via JetStream PublishAsync. Returns an error when marshalling fails or
// when the JetStream publish fails.
//
// The subject must be in the "sys.in." namespace to match the SYSTEM_EVENTS
// stream. Subjects outside this namespace are not captured by the stream and
// will not be persisted.
func (n *NATS) Publish(subject string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("protobuf marshal failed: %w", err)
	}

	_, err = n.JS.PublishAsync(subject, data)
	return err
}

// Subscribe registers a core NATS subscription (not JetStream pull-based) on
// the given subject pattern. Returns an error when the connection is nil or
// when nats.Conn.Subscribe fails.
//
// The handler receives messages as they arrive. The subscription is not
// durable and is tied to the connection lifetime. Use JetStream pull
// subscriptions for durable, queue-based consumption.
func (n *NATS) Subscribe(subject string, handler nats.MsgHandler) (*nats.Subscription, error) {
	if n == nil || n.Conn == nil {
		return nil, fmt.Errorf("NATS connection is nil; call New before Subscribe")
	}

	return n.Conn.Subscribe(subject, handler)
}

// ensureSystemEventsStream creates the SYSTEM_EVENTS stream when it does not
// exist. The stream covers the "sys.in.>" subject namespace and uses file
// storage for persistence.
func ensureSystemEventsStream(js nats.JetStreamContext) error {
	_, err := js.StreamInfo(systemEventsStream)
	if err == nil {
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("%s stream lookup failed: %w", systemEventsStream, err)
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     systemEventsStream,
		Subjects: []string{systemEventsSubject},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("%s stream creation failed: %w", systemEventsStream, err)
	}

	return nil
}
