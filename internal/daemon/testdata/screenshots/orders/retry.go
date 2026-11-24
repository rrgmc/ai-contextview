package orders

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// RetryPolicy decides how often a failed webhook delivery is retried.
type RetryPolicy struct {
	First       time.Duration // wait before the first retry
	Max         time.Duration // cap for the wait between retries
	MaxAttempts int           // attempts before the event is dead-lettered
}

// DefaultRetryPolicy is the policy from the spec.
var DefaultRetryPolicy = RetryPolicy{
	First:       time.Second,
	Max:         30 * time.Second,
	MaxAttempts: 8,
}

// Backoff returns the wait before retry n (1-based).
func (p RetryPolicy) Backoff(n int) time.Duration {
	d := p.First << (n - 1)
	if d <= 0 || d > p.Max {
		return p.Max
	}
	return d
}

// ErrFinal marks a failure that is not retried.
var ErrFinal = errors.New("final delivery failure")

// classify turns an HTTP status into a retryable or final error.
func classify(status int) error {
	switch {
	case status < 400:
		return nil
	case status == http.StatusTooManyRequests, status >= 500:
		return errors.New(http.StatusText(status))
	default:
		return ErrFinal
	}
}

// Deliver sends one event and retries it according to the policy.
func (p RetryPolicy) Deliver(ctx context.Context, send func(context.Context) (int, error)) error {
	var err error
	for n := 1; n <= p.MaxAttempts; n++ {
		var status int
		status, err = send(ctx)
		if err == nil {
			err = classify(status)
		}
		if err == nil || errors.Is(err, ErrFinal) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.Backoff(n)):
		}
	}
	return err
}
