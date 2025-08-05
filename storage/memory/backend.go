package memory

import (
	"context"

	"github.com/ryosan-470/tokenbucket/storage"
)

type Backend struct {
	state storage.State
}

func NewBackend() *Backend {
	return &Backend{}
}

func (b *Backend) State(ctx context.Context) (storage.State, error) {
	return b.state, nil
}

func (b *Backend) SetState(ctx context.Context, state storage.State) error {
	b.state = state
	return nil
}

func (b *Backend) Reset(ctx context.Context) error {
	b.state = storage.NewState(0, 0)
	return nil
}
