package tokenbucket

import (
	"context"
	"sync"

	"github.com/ryosan-470/tokenbucket/internal/clock"
	"github.com/ryosan-470/tokenbucket/storage"
)

type TokenBucket interface {
	// Take attempts to take a token from the bucket. If successful, it returns nil.
	// If no tokens are available, it returns an ErrNoTokensAvailable error.
	Take(ctx context.Context) error

	// Get returns the current state of the bucket, including available tokens and last updated timestamp.
	Get(ctx context.Context) (*Bucket, error)
}

var _ TokenBucket = (*Bucket)(nil)

type Bucket struct {
	Capacity    int64  // Maximum number of tokens in the bucket
	FillRate    int64  // Rate at which tokens are added to the bucket (tokens per second)
	Available   int64  // Current number of available tokens
	LastUpdated int64  // Timestamp of the last update in milliseconds
	Dimension   string // Dimension of the token bucket

	backend storage.Storage
	clock   clock.Clock  // Clock for time-related operations
	mu      sync.RWMutex // RWMutex to protect concurrent access to the bucket state
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

func NewBucket(capacity, fillRate int64, dimension string, backend storage.Storage, opts ...Option) (*Bucket, error) {
	opt := &options{
		clock: clock.NewSystemClock(),
	}
	for _, o := range opts {
		o(opt)
	}

	return &Bucket{
		Capacity:  capacity,
		FillRate:  fillRate,
		Available: capacity,
		Dimension: dimension,

		backend: backend,
		clock:   opt.clock,
		mu:      sync.RWMutex{},
	}, nil
}

func (b *Bucket) Take(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	state, err := b.backend.State(ctx)
	if err != nil {
		return err
	}

	if state.IsEmpty() {
		// Initialize the bucket state if it's empty
		state = storage.NewState(b.Capacity, b.clock.Now().UnixNano())
	}

	// Refill tokens based on the elapsed time since the last update
	now := b.clock.Now().UnixNano()
	tokenToAdd := (now - state.Last) * b.FillRate / 1e9 // Convert nanoseconds to seconds
	if tokenToAdd > 0 {
		state.Available += tokenToAdd
		if state.Available > b.Capacity {
			state.Available = b.Capacity
		}
		state.Last = now
	}

	// Take a token if available
	if state.Available <= 0 {
		return ErrNoTokensAvailable
	}

	state.Available--
	if err := b.backend.SetState(ctx, state); err != nil {
		return err
	}

	b.Available = state.Available
	b.LastUpdated = state.Last
	return nil
}

func (b *Bucket) Get(ctx context.Context) (*Bucket, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	state, err := b.backend.State(ctx)
	if err != nil {
		return nil, err
	}

	return &Bucket{
		Capacity:    b.Capacity,
		FillRate:    b.FillRate,
		Available:   state.Available,
		LastUpdated: state.Last,
		Dimension:   b.Dimension,
		backend:     b.backend,
		clock:       b.clock,
	}, nil
}
