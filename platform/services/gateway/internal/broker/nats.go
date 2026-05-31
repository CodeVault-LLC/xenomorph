package broker

import (
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

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "SYSTEM_EVENTS",
		Subjects: []string{"sys.in.>"},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("stream creation: %w", err)
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
