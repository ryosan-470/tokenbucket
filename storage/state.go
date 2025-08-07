package storage

import (
	"context"
)

// State represents the state of a token bucket
type State struct {
	Available int64 // Number of available tokens
	Last      int64 // Last update timestamp in nanoseconds
}

func NewState(available int64, last int64) State {
	return State{
		Available: available,
		Last:      last,
	}
}

func (s State) IsEmpty() bool {
	return s.Available == 0 && s.Last == 0
}

// Storage defines the common interface for token bucket storage backends
type Storage interface {
	// State gets the current state of the TokenBucket
	State(ctx context.Context) (State, error)
	// SetState sets (persists) the current state of the TokenBucket
	SetState(ctx context.Context, state State) error
	// Reset resets (persists) the current state of the TokenBucket
	Reset(ctx context.Context) error
}
