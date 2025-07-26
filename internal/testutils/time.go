package testutils

import (
	"time"
)

// MockClock implements limiters.Clock interface for testing
type MockClock struct {
	currentTime time.Time
}

func NewMockClock(startTime time.Time) *MockClock {
	return &MockClock{currentTime: startTime}
}

func (m *MockClock) Now() time.Time {
	return m.currentTime
}

func (m *MockClock) Advance(duration time.Duration) {
	m.currentTime = m.currentTime.Add(duration)
}
