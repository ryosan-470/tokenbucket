package dynamodb

import (
	"context"

	"github.com/mennanov/limiters"
)

var _ limiters.DistLocker = (*Lock)(nil)

type Lock struct{}

func NewLock() *Lock {
	panic("not implemented")
}

func (l *Lock) Lock(ctx context.Context) error {
	panic("not implemented")
}

func (l *Lock) Unlock(ctx context.Context) error {
	panic("not implemented")
}
