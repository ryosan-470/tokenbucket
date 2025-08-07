package clock

import "time"

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func NewSystemClock() Clock {
	return &SystemClock{}
}

func (c *SystemClock) Now() time.Time {
	return time.Now()
}
