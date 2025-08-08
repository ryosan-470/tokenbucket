package memory

import (
	"context"
	"sync"

	"github.com/ryosan-470/tokenbucket"
)

type MemoryBackend struct {
	Available   int64 // Number of available tokens
	LastUpdated int64 // Last update timestamp in nanoseconds

	mu sync.RWMutex // Mutex to protect concurrent access
}

var _ tokenbucket.TokenBucketStateRepository = (*MemoryBackend)(nil)

func NewBackend() *MemoryBackend {
	return &MemoryBackend{}
}

func (m *MemoryBackend) State(ctx context.Context) (tokenbucket.State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return tokenbucket.NewState(m.Available, m.LastUpdated), nil
}

func (m *MemoryBackend) SetState(ctx context.Context, state tokenbucket.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Available = state.Available
	m.LastUpdated = state.LastUpdated
	return nil
}

func (m *MemoryBackend) Reset(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Available = 0
	m.LastUpdated = 0
	return nil
}
