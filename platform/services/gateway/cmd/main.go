package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/transport"
)

func main() {
	natsBroker, err := broker.New("nats://localhost:4222")
	if err != nil {
		log.Fatalf("❌ Failed to connect to NATS: %v", err)
	}
	defer natsBroker.Close()
	log.Println("✅ Connected to NATS JetStream")

	srv := transport.NewServer(natsBroker)

	certPath := "../../infrastructure/certs"

	go func() {
		log.Println("🔒 Gateway listening on :8443 (mTLS Required)")
		if err := srv.Run(":8443", certPath); err != nil {
			log.Fatalf("❌ Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Gateway...")
}
