package broker

import (
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

type NATS struct {
	Conn *nats.Conn
	JS   nats.JetStreamContext
}

func New(url string) (*NATS, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("jetstream init: %w", err)
	}

	if err := ensureSystemEventsStream(js); err != nil {
		return nil, err
	}

	return &NATS{Conn: nc, JS: js}, nil
}

func (n *NATS) Close() {
	if n.Conn != nil {
		n.Conn.Close()
	}
}

func (n *NATS) Publish(subject string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("proto marshal: %w", err)
	}

	_, err = n.JS.PublishAsync(subject, data)
	return err
}

func (n *NATS) Subscribe(subject string, handler nats.MsgHandler) (*nats.Subscription, error) {
	if n == nil || n.Conn == nil {
		return nil, fmt.Errorf("nats connection is not initialized")
	}

	return n.Conn.Subscribe(subject, handler)
}

func ensureSystemEventsStream(js nats.JetStreamContext) error {
	_, err := js.StreamInfo("SYSTEM_EVENTS")
	if err == nil {
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("stream lookup: %w", err)
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "SYSTEM_EVENTS",
		Subjects: []string{"sys.in.>"},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("stream creation: %w", err)
	}

	return nil
}
