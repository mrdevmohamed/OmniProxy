package tunnel

import (
	"context"
	"net"
	"strconv"
	"time"

	"omniproxy/core/log"
	"omniproxy/core/models"
)

// DefaultLatencyTimeout bounds a single TCP dial.
const DefaultLatencyTimeout = 3 * time.Second

// LatencyTester measures server reachability by TCP-dialing the endpoint.
// Plan decision: core TCP-dial (not sing-box url-test) for Phase 1.
type LatencyTester struct {
	timeout time.Duration
	logger  *log.Logger
}

// NewLatencyTester returns a tester with the given dial timeout
// (DefaultLatencyTimeout when <= 0).
func NewLatencyTester(timeout time.Duration, logger *log.Logger) *LatencyTester {
	if timeout <= 0 {
		timeout = DefaultLatencyTimeout
	}
	return &LatencyTester{timeout: timeout, logger: logger}
}

// TestLatency implements server.LatencyTester.
func (t *LatencyTester) TestLatency(ctx context.Context, p *models.ServerProfile) (int, error) {
	addr := net.JoinHostPort(p.Address, strconv.Itoa(p.Port))
	dialer := &net.Dialer{Timeout: t.timeout}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, err
	}
	conn.Close()
	ms := int(time.Since(start).Milliseconds())
	if ms < 1 {
		ms = 1
	}
	t.logger.Infof("tunnel", "latency to %s:%d = %dms", p.Address, p.Port, ms)
	return ms, nil
}
