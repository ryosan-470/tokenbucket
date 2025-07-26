package tokenbucket

import (
	"context"
	"time"

	"github.com/mennanov/limiters"

	"github.com/ryosan-470/tokenbucket/storage/dynamodb"
)

type TokenBucket interface {
	// Take attempts to take a token from the bucket. If successful, it returns nil.
	Take(ctx context.Context) error

	// Get attempts to get the current state of the bucket, including available tokens and last updated timestamp.
	Get(ctx context.Context) (*Bucket, error)
}

type Bucket struct {
	Capacity    int64  // Maximum number of tokens in the bucket
	FillRate    int64  // Rate at which tokens are added to the bucket (tokens per second)
	Available   int64  // Current number of available tokens
	LastUpdated int64  // Timestamp of the last update in milliseconds
	Dimension   string // Dimension of the token bucket

	backend dynamodb.BucketBackendInterface
	bucket  *limiters.TokenBucket // Underlying token bucket implementation
	lock    limiters.DistLocker   // DynamoDB lock for distributed coordination
}

type options struct {
	clock   limiters.Clock
	logger  limiters.Logger
	lock    limiters.DistLocker
	backend dynamodb.BucketBackendInterface
}

type Option func(*options)

func WithClock(c limiters.Clock) Option {
	return func(o *options) {
		o.clock = c
	}
}

func WithLogger(l limiters.Logger) Option {
	return func(o *options) {
		o.logger = l
	}
}

func WithLockBackend(cfg *dynamodb.LockBackendConfig, lockID string) Option {
	return func(o *options) {
		o.lock = cfg.NewLockBackend(lockID)
	}
}

func WithLimitersBackend(cfg *dynamodb.BucketBackendConfig, enableRaceHandling bool) Option {
	backend, _ := cfg.NewLimitersBackend(context.Background(), enableRaceHandling)
	return func(o *options) {
		o.backend = backend
	}
}

func NewBucket(capacity, fillRate int64, dimension string, backendConfig *dynamodb.BucketBackendConfig, opts ...Option) (*Bucket, error) {
	if backendConfig == nil {
		return nil, ErrInitializedBucketFailed
	}
	opt := &options{
		clock:  limiters.NewSystemClock(),
		logger: &limiters.StdLogger{},
	}
	for _, o := range opts {
		o(opt)
	}

	var backend dynamodb.BucketBackendInterface
	if opt.backend != nil {
		backend = opt.backend
	} else {
		var err error
		backend, err = backendConfig.NewCustomBackend(context.Background())
		if err != nil {
			return nil, ErrInitializedBucketFailed
		}
	}

	var lock limiters.DistLocker
	if opt.lock != nil {
		lock = opt.lock
	} else {
		lock = limiters.NewLockNoop()
	}

	bucket := limiters.NewTokenBucket(
		capacity,
		calculateFillRate(fillRate),
		lock,
		backend,
		opt.clock,
		opt.logger,
	)

	return &Bucket{
		Capacity:  capacity,
		FillRate:  fillRate,
		Available: capacity,
		Dimension: dimension,
		backend:   backend,
		bucket:    bucket,
		lock:      lock,
	}, nil
}

func (b *Bucket) Take(ctx context.Context) error {
	if _, err := b.bucket.Take(ctx, 1); err != nil {
		return err
	}
	return nil
}

func (b *Bucket) Get(ctx context.Context) (*Bucket, error) {
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
		bucket:      b.bucket,
		lock:        b.lock,
	}, nil
}

func calculateFillRate(fillRate int64) time.Duration {
	if fillRate <= 0 {
		return 0
	}
	return time.Second / time.Duration(fillRate)
}
