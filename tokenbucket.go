package tokenbucket

import (
	"context"
	"fmt"
	"time"

	"github.com/mennanov/limiters"

	"github.com/ryosan-470/tokenbucket/storage/dynamodb"
)

const (
	lockIDPrefix = "bucket_lock_"
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

	backend *limiters.TokenBucketDynamoDB // Backend of the token bucket
	bucket  *limiters.TokenBucket         // Underlying token bucket implementation
	lock    limiters.DistLocker           // DynamoDB lock for distributed coordination
}

type options struct {
	clock       limiters.Clock
	logger      limiters.Logger
	disableLock bool
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

func WithoutLock() Option {
	return func(o *options) {
		o.disableLock = true
	}
}

func NewBucket(capacity, fillRate int64, dimension string, cfg *dynamodb.BucketBackendConfig, opts ...Option) (*Bucket, error) {
	opt := &options{
		clock:  limiters.NewSystemClock(),
		logger: &limiters.StdLogger{},
	}
	for _, o := range opts {
		o(opt)
	}

	backend, err := cfg.NewTokenBucketDynamoDB(context.Background())
	if err != nil {
		return nil, ErrInitializedBucketFailed
	}

	var lock limiters.DistLocker
	if !opt.disableLock {
		lockID := fmt.Sprintf("%s%s", lockIDPrefix, dimension)
		lock = cfg.NewLock(lockID)
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
