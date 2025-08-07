package dynamodb

import (
	"context"

	"github.com/mennanov/limiters"

	"github.com/ryosan-470/tokenbucket/storage"
)

// LimitersBackendAdapter adapts limiters.TokenBucketStateBackend to storage.BucketBackendInterface
type LimitersBackendAdapter struct {
	backend limiters.TokenBucketStateBackend
}

// NewLimitersBackendAdapter creates a new adapter that wraps a limiters backend
func NewLimitersBackendAdapter(backend limiters.TokenBucketStateBackend) storage.Storage {
	return &LimitersBackendAdapter{
		backend: backend,
	}
}

func (a *LimitersBackendAdapter) State(ctx context.Context) (storage.State, error) {
	state, err := a.backend.State(ctx)
	if err != nil {
		return storage.State{}, err
	}

	return storage.State{
		Available: state.Available,
		Last:      state.Last,
	}, nil
}

func (a *LimitersBackendAdapter) SetState(ctx context.Context, state storage.State) error {
	limitersState := limiters.TokenBucketState{
		Available: state.Available,
		Last:      state.Last,
	}
	return a.backend.SetState(ctx, limitersState)
}

func (a *LimitersBackendAdapter) Reset(ctx context.Context) error {
	return a.backend.Reset(ctx)
}

// StorageBackendAdapter adapts storage.BucketBackendInterface to limiters.TokenBucketStateBackend
type StorageBackendAdapter struct {
	backend storage.Storage
}

// NewStorageBackendAdapter creates a new adapter that wraps a storage backend
func NewStorageBackendAdapter(backend storage.Storage) limiters.TokenBucketStateBackend {
	return &StorageBackendAdapter{
		backend: backend,
	}
}

func (a *StorageBackendAdapter) State(ctx context.Context) (limiters.TokenBucketState, error) {
	state, err := a.backend.State(ctx)
	if err != nil {
		return limiters.TokenBucketState{}, err
	}

	return limiters.TokenBucketState{
		Available: state.Available,
		Last:      state.Last,
	}, nil
}

func (a *StorageBackendAdapter) SetState(ctx context.Context, state limiters.TokenBucketState) error {
	storageState := storage.State{
		Available: state.Available,
		Last:      state.Last,
	}
	return a.backend.SetState(ctx, storageState)
}

func (a *StorageBackendAdapter) Reset(ctx context.Context) error {
	return a.backend.Reset(ctx)
}
