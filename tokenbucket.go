package tokenbucket

import (
	"context"
	"sync"

	"github.com/ryosan-470/tokenbucket/internal/clock"
)

type TokenBucket interface {
	// Take attempts to take a token from the bucket. If successful, it returns nil.
	// If no tokens are available, it returns an ErrNoTokensAvailable error.
	Take(ctx context.Context) error

	// GetState returns the current state of the bucket, including available tokens and last updated timestamp.
	GetState(ctx context.Context) (State, error)
}

var _ TokenBucket = (*Bucket)(nil)

type Bucket struct {
	Capacity  int64  // Maximum number of tokens in the bucket
	FillRate  int64  // Rate at which tokens are added to the bucket (tokens per second)
	Dimension string // Dimension of the token bucket

	r     TokenBucketStateRepository
	clock clock.Clock  // Clock for time-related operations
	mu    sync.RWMutex // RWMutex to protect concurrent access to the bucket state
}

type options struct {
	clock clock.Clock
}

type Option func(*options)

func WithClock(c clock.Clock) Option {
	return func(o *options) {
		o.clock = c
	}
}

func NewBucket(capacity, fillRate int64, dimension string, r TokenBucketStateRepository, opts ...Option) (*Bucket, error) {
	opt := &options{
		clock: clock.NewSystemClock(),
	}
	for _, o := range opts {
		o(opt)
	}

	return &Bucket{
		Capacity:  capacity,
		FillRate:  fillRate,
		Dimension: dimension,

		r:     r,
		clock: opt.clock,
		mu:    sync.RWMutex{},
	}, nil
}

func (b *Bucket) Take(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	state, err := b.r.State(ctx)
	if err != nil {
		return err
	}

	if state.IsEmpty() {
		// Initialize the bucket state if it's empty
		state = NewState(b.Capacity, b.clock.Now().UnixNano())
	}

	// Refill tokens based on the elapsed time since the last update
	now := b.clock.Now().UnixNano()
	tokenToAdd := (now - state.LastUpdated) * b.FillRate / 1e9 // Convert nanoseconds to seconds
	if tokenToAdd > 0 {
		state.Available += tokenToAdd
		if state.Available > b.Capacity {
			state.Available = b.Capacity
		}
		state.LastUpdated = now
	}

	// Take a token if available
	if state.Available <= 0 {
		return ErrNoTokensAvailable
	}

	state.Available--
	if err := b.r.SetState(ctx, state); err != nil {
		return err
	}

	return nil
}

// GetState returns the current state of the bucket including available tokens and last updated timestamp
func (b *Bucket) GetState(ctx context.Context) (State, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	s, err := b.r.State(ctx)
	if err != nil {
		return State{}, err
	}

	return State{
		Available:   s.Available,
		LastUpdated: s.LastUpdated,
	}, nil
}
