package storage

import (
	"context"

	"github.com/ryosan-470/tokenbucket"
)

// Provider provides TokenBucket instances for benchmarking
type Provider interface {
	Setup(ctx context.Context) error
	Cleanup(ctx context.Context) error
	CreateBucket(capacity, fillRate int64, dimension string, opts ...tokenbucket.Option) (*tokenbucket.Bucket, error)
}