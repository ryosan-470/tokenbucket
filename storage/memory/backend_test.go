package memory_test

import (
	"context"
	"testing"

	"github.com/ryosan-470/tokenbucket"
	"github.com/ryosan-470/tokenbucket/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryBackend(t *testing.T) {
	backend := memory.NewBackend()

	t.Run("InitialState", func(t *testing.T) {
		state, err := backend.State(context.Background())
		require.NoError(t, err)
		assert.Equal(t, tokenbucket.NewState(0, 0), state)
	})

	t.Run("SetState", func(t *testing.T) {
		newState := tokenbucket.NewState(100, 50)
		err := backend.SetState(context.Background(), newState)
		require.NoError(t, err)

		state, err := backend.State(context.Background())
		require.NoError(t, err)
		assert.Equal(t, newState, state)
	})

	t.Run("Reset", func(t *testing.T) {
		err := backend.Reset(context.Background())
		require.NoError(t, err)

		state, err := backend.State(context.Background())
		require.NoError(t, err)
		assert.Equal(t, tokenbucket.NewState(0, 0), state)
	})
}
