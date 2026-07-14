package broker

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeJetStream struct {
	streamInfo    *nats.StreamInfo
	streamInfoErr error
	addedConfig   *nats.StreamConfig
	addErr        error
	published     []byte
	publishCalled bool
	publishErr    error
}

func (f *fakeJetStream) Publish(_ string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	f.publishCalled = true
	f.published = append([]byte(nil), data...)
	return &nats.PubAck{Stream: systemEventsStream}, f.publishErr
}

func (f *fakeJetStream) StreamInfo(_ string, _ ...nats.JSOpt) (*nats.StreamInfo, error) {
	return f.streamInfo, f.streamInfoErr
}

func (f *fakeJetStream) AddStream(config *nats.StreamConfig, _ ...nats.JSOpt) (*nats.StreamInfo, error) {
	f.addedConfig = config
	return &nats.StreamInfo{Config: *config}, f.addErr
}

func TestEnsureSystemEventsStream(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		jetStream  *fakeJetStream
		wantConfig *nats.StreamConfig
		wantError  bool
	}{
		{name: "existing stream", jetStream: &fakeJetStream{streamInfo: &nats.StreamInfo{}}},
		{
			name: "creates missing stream", jetStream: &fakeJetStream{streamInfoErr: nats.ErrStreamNotFound},
			wantConfig: &nats.StreamConfig{Name: systemEventsStream, Subjects: []string{systemEventsSubject}, Storage: nats.FileStorage},
		},
		{name: "lookup failure", jetStream: &fakeJetStream{streamInfoErr: errors.New("lookup failed")}, wantError: true},
		{
			name: "creation failure", jetStream: &fakeJetStream{streamInfoErr: nats.ErrStreamNotFound, addErr: errors.New("create failed")},
			wantConfig: &nats.StreamConfig{Name: systemEventsStream, Subjects: []string{systemEventsSubject}, Storage: nats.FileStorage}, wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ensureSystemEventsStream(test.jetStream)
			if (err != nil) != test.wantError {
				t.Fatalf("ensureSystemEventsStream() error = %v, wantError %t", err, test.wantError)
			}
			if !reflect.DeepEqual(test.jetStream.addedConfig, test.wantConfig) {
				t.Errorf("created config = %#v, want %#v", test.jetStream.addedConfig, test.wantConfig)
			}
		})
	}
}

func TestPublishWaitsForJetStreamAcknowledgement(t *testing.T) {
	t.Parallel()
	jetStream := &fakeJetStream{}
	broker := &NATS{js: jetStream}
	if err := broker.Publish("sys.in.test", &emptypb.Empty{}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if !jetStream.publishCalled {
		t.Fatal("Publish() did not call the synchronous JetStream publisher")
	}
}

func TestPublishRejectsUnavailableJetStream(t *testing.T) {
	t.Parallel()
	if err := (&NATS{}).Publish("sys.in.test", &emptypb.Empty{}); err == nil {
		t.Fatal("Publish() error = nil, want unavailable JetStream error")
	}
}

func TestPublishReturnsAcknowledgementFailure(t *testing.T) {
	t.Parallel()
	broker := &NATS{js: &fakeJetStream{publishErr: errors.New("no acknowledgement")}}
	if err := broker.Publish("sys.in.test", &emptypb.Empty{}); err == nil {
		t.Fatal("Publish() error = nil, want acknowledgement error")
	}
}
