module github.com/codevault-llc/xenomorph/platform/client

go 1.25.3

require (
	github.com/codevault-llc/xenomorph/platform/shared v0.0.0
	github.com/gorilla/websocket v1.4.2
	golang.org/x/sys v0.42.0
)

replace github.com/codevault-llc/xenomorph/platform/shared => ../shared
