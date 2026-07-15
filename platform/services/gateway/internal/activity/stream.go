package activity

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
)

// StartStream subscribes to agent heartbeat events on NATS and starts the
// periodic sweep that transitions stale agents to offline state.
//
// The function subscribes to the wildcard pattern "sys.in.default.*.heartbeat"
// on the NATS broker. Each incoming message is unmarshalled as a
// proto.EventEnvelope and forwarded to ProcessHeartbeat. A background
// goroutine ticks at sweepInterval and calls Sweep to detect agent timeouts.
//
// The subscription and sweep goroutine both honour ctx cancellation. Callers
// must cancel ctx during graceful shutdown (e.g. on SIGINT/SIGTERM).
//
// Security: the subscription pattern is restricted to the "sys.in.default."
// namespace. This prefix is the gateway-controlled ingress namespace. No
// external client can publish to subjects matching this pattern without
// going through the gateway's mTLS-authenticated HTTP endpoints.
func StartStream(ctx context.Context, natsBroker *broker.NATS, monitor *Monitor, sweepInterval time.Duration) error {
	_, err := natsBroker.Subscribe("sys.in.default.*.heartbeat", func(msg *nats.Msg) {
		var envelope pb.EventEnvelope
		if err := proto.Unmarshal(msg.Data, &envelope); err != nil {
			slog.ErrorContext(ctx, "failed to decode heartbeat envelope",
				"error", err,
				"subject", msg.Subject,
			)

			return
		}

		if err := monitor.ProcessHeartbeat(ctx, &envelope); err != nil {
			slog.ErrorContext(ctx, "failed to process heartbeat",
				"error", err,
				"agent_id", envelope.Security.GetAgentId(),
			)
		}
	})
	if err != nil {
		return err
	}

	ticker := time.NewTicker(sweepInterval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := monitor.Sweep(ctx); err != nil {
					slog.ErrorContext(ctx, "activity sweep failed",
						"error", err,
					)
				}
			}
		}
	}()

	return nil
}
