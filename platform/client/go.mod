module github.com/codevault-llc/xenomorph/platform/client

go 1.25.12

require (
	github.com/codevault-llc/xenomorph/platform/shared v0.0.0
	github.com/google/uuid v1.6.0
	github.com/quic-go/quic-go v0.61.0
	golang.org/x/sys v0.47.0
)

require (
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/net v0.56.0 // indirect
)

replace github.com/codevault-llc/xenomorph/platform/shared => ../shared
