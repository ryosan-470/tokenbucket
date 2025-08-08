package tokenbucket

import "context"

type State struct {
	Available   int64 // Number of available tokens
	LastUpdated int64 // Last update timestamp in nanoseconds
}

// NewState creates a new State with the given available tokens and last updated timestamp
func NewState(available, lastUpdated int64) State {
	return State{
		Available:   available,
		LastUpdated: lastUpdated,
	}
}

// IsEmpty returns true if the state is uninitialized (no tokens and no last updated time)
func (s State) IsEmpty() bool {
	return s.Available == 0 && s.LastUpdated == 0
}

type TokenBucketStateRepository interface {
	// State retrieves the current state of the token bucket
	State(ctx context.Context) (State, error)

	// SetState persists the current state of the token bucket
	SetState(ctx context.Context, state State) error

	// Reset resets the state of the token bucket
	Reset(ctx context.Context) error
}
