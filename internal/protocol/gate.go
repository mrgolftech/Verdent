package protocol

import (
	"context"
	"sync"
	"time"
)

type RequestGate struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
}

func NewRequestGate(interval time.Duration) *RequestGate {
	if interval < 0 {
		interval = 0
	}
	return &RequestGate{interval: interval}
}

func (g *RequestGate) Wait(ctx context.Context) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.last.IsZero() && g.interval > 0 {
		wait := g.interval - time.Since(g.last)
		if wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	g.last = time.Now()
	return nil
}
