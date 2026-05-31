package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
)

const (
	GatewayURL = "https://localhost:8443"
	CertPath   = "../infrastructure/certs"
)

func main() {
	cert, err := tls.LoadX509KeyPair(CertPath+"/client.crt", CertPath+"/client.key")
	if err != nil {
		log.Fatalf("❌ Failed to load client certs: %v", err)
	}

	caCert, err := ioutil.ReadFile(CertPath + "/ca.crt")
	if err != nil {
		log.Fatalf("❌ Failed to read CA cert: %v", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	a := agent.New(httpClient, GatewayURL)

	log.Println("✅ Agent initialized. Starting heartbeat loop...")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("💓 Sending Heartbeat...")

		if err := a.SendHeartbeat(); err != nil {
			log.Printf("⚠️ Heartbeat failed: %v", err)
		} else {
			log.Printf("👍 Heartbeat acknowledged")
		}
	}
}
