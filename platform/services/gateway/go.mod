module github.com/codevault-llc/xenomorph/platform/services/gateway

go 1.25.12

require (
	github.com/codevault-llc/xenomorph/platform/shared v0.0.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.4.2
	github.com/nats-io/nats.go v1.47.0
	github.com/quic-go/quic-go v0.60.0
	google.golang.org/protobuf v1.36.10
)

require (
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

replace github.com/codevault-llc/xenomorph/platform/shared => ../../shared
