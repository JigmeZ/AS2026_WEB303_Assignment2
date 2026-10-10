package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	itempb "lostfound/gen/itempb"
)

// ItemClientConfig holds the resilience settings (all loaded from env vars).
type ItemClientConfig struct {
	Timeout          time.Duration // per-attempt timeout
	MaxAttempts      int           // total attempts (1 original + retries)
	BaseBackoff      time.Duration // first backoff; doubles every retry
	FailureThreshold int           // consecutive failed calls before the breaker opens
	OpenDuration     time.Duration // how long the breaker stays open before a trial call
}

// ItemClient wraps the generated gRPC client with:
//   1. a timeout on every attempt,
//   2. retries with exponential backoff (only for transient errors),
//   3. a circuit breaker around the whole retried call.
type ItemClient struct {
	client  itempb.ItemServiceClient
	breaker *gobreaker.CircuitBreaker
	cfg     ItemClientConfig
}

func NewItemClient(client itempb.ItemServiceClient, cfg ItemClientConfig) *ItemClient {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "item-service",
		MaxRequests: 1, // while half-open, allow one trial request
		Timeout:     cfg.OpenDuration,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return int(c.ConsecutiveFailures) >= cfg.FailureThreshold
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Printf("CIRCUIT BREAKER [%s]: %s -> %s", name, from, to)
		},
	})
	return &ItemClient{client: client, breaker: cb, cfg: cfg}
}

// callResult lets us pass "business" errors (e.g. NotFound) through the breaker
// without counting them as failures. Only infrastructure errors trip the breaker.
type callResult struct {
	item *itempb.Item
	err  error
}

// isTransient reports whether an error is worth retrying / counts toward the breaker.
func isTransient(err error) bool {
	c := status.Code(err)
	return c == codes.Unavailable || c == codes.DeadlineExceeded
}

// GetItem fetches an item from Service B with all resilience patterns applied.
func (c *ItemClient) GetItem(ctx context.Context, id int64) (*itempb.Item, error) {
	res, err := c.breaker.Execute(func() (interface{}, error) {
		item, err := c.getWithRetry(ctx, id)
		if err != nil && !isTransient(err) {
			return callResult{err: err}, nil // e.g. NotFound: Service B is healthy
		}
		if err != nil {
			return nil, err // counts as a breaker failure
		}
		return callResult{item: item}, nil
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			log.Printf("item-service call rejected immediately: circuit breaker is open")
			return nil, status.Error(codes.Unavailable,
				"item service is unavailable (circuit breaker open), please try again shortly")
		}
		return nil, err
	}
	r := res.(callResult)
	return r.item, r.err
}

// getWithRetry performs the call with a per-attempt timeout and exponential backoff.
func (c *ItemClient) getWithRetry(ctx context.Context, id int64) (*itempb.Item, error) {
	var lastErr error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		item, err := c.client.GetItem(attemptCtx, &itempb.GetItemRequest{Id: id})
		cancel()

		if err == nil {
			return item, nil
		}
		if !isTransient(err) {
			return nil, err // NotFound, InvalidArgument... retrying will not help
		}

		lastErr = err
		log.Printf("item-service call failed (attempt %d/%d): %v", attempt, c.cfg.MaxAttempts, err)

		if attempt < c.cfg.MaxAttempts {
			backoff := c.cfg.BaseBackoff * time.Duration(1<<(attempt-1)) // 200ms, 400ms, 800ms...
			log.Printf("retrying in %s", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, status.FromContextError(ctx.Err()).Err()
			}
		}
	}
	return nil, status.Errorf(codes.Unavailable,
		"item service unavailable after %d attempts: %s",
		c.cfg.MaxAttempts, status.Convert(lastErr).Message())
}
