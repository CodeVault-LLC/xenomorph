package transport

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
)

type Server struct {
	broker *broker.NATS
	engine *gin.Engine
}

func NewServer(b *broker.NATS) *Server {
	s := &Server{broker: b, engine: gin.Default()}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.engine.Use(func(c *gin.Context) {
		if c.Request.TLS != nil && len(c.Request.TLS.PeerCertificates) > 0 {
			cert := c.Request.TLS.PeerCertificates[0]
			c.Set("agent_id", cert.Subject.CommonName)
		} else {
			c.AbortWithStatusJSON(403, gin.H{"error": "mTLS required"})
			return
		}
		c.Next()
	})

	s.engine.POST("/ingest/heartbeat", s.handleHeartbeat)
}

func (s *Server) handleHeartbeat(c *gin.Context) {
	agentID := c.GetString("agent_id")

	var hb pb.Heartbeat
	if err := c.ShouldBindJSON(&hb); err != nil {
		c.JSON(400, gin.H{"error": "Invalid schema"})
		return
	}

	// Construct Trusted Envelope
	env := &pb.EventEnvelope{
		EventId:   uuid.New().String(),
		TraceId:   c.GetHeader("X-Trace-ID"), // Or generate new
		Timestamp: timestamppb.Now(),
		Security: &pb.SecurityContext{
			AgentId:         agentID,
			SessionId:       uuid.New().String(), // In real app, cache this
			ClientIp:        c.ClientIP(),
			IsAuthenticated: true,
		},
		Payload: &pb.EventEnvelope_Heartbeat{Heartbeat: &hb},
	}

	// Publish to NATS: sys.in.us-east.agent-555.heartbeat
	subject := "sys.in.default." + agentID + ".heartbeat"
	if err := s.broker.Publish(subject, env); err != nil {
		c.JSON(500, gin.H{"error": "Failed to queue event"})
		return
	}

	c.JSON(202, gin.H{"status": "accepted", "event_id": env.EventId})
}

func (s *Server) Run(addr, certPath string) error {
	// Load CA to verify clients
	caCert, _ := ioutil.ReadFile(certPath + "/ca.crt")
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		ClientCAs:  caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert, // STRICT mTLS
		MinVersion: tls.VersionTLS13,
	}

	server := &http.Server{
		Addr:      addr,
		Handler:   s.engine,
		TLSConfig: tlsConfig,
	}
	return server.ListenAndServeTLS(certPath+"/server.crt", certPath+"/server.key")
}
