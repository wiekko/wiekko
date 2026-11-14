package mail

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrLimitReached is returned while the instance has used up its messages for
// the hour.
var ErrLimitReached = errors.New("mail: hourly sending limit reached")

// capped is a circuit breaker around a Sender: whatever the per-user and
// per-address limits upstream let through, the instance never sends more than
// max messages an hour. A bug or an unforeseen abuse path then costs at most
// that many messages, not the reputation of the sending domain.
type capped struct {
	Sender
	max int

	mu          sync.Mutex
	windowStart time.Time
	count       int
	now         func() time.Time
}

// NewCapped limits s to max messages per hour; max <= 0 means
// DefaultMaxPerHour.
func NewCapped(s Sender, max int) Sender {
	if max <= 0 {
		max = DefaultMaxPerHour
	}
	return &capped{Sender: s, max: max, now: time.Now}
}

// Send counts every attempt, failed or not, and refuses once the hour's
// messages are used up.
func (c *capped) Send(ctx context.Context, msg Message) error {
	if err := c.take(); err != nil {
		return err
	}
	return c.Sender.Send(ctx, msg)
}

func (c *capped) take() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if now.Sub(c.windowStart) >= time.Hour {
		c.windowStart, c.count = now, 0
	}
	if c.count >= c.max {
		return ErrLimitReached
	}
	c.count++
	return nil
}
