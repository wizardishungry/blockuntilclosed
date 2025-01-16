package blockuntilclosed

import (
	"context"
	"sync"
)

type Backend interface {
	Block(ctx context.Context, fd uintptr) error
	IsClosed(fd uintptr) (bool, error)
}

var (
	defaultBackendOnce sync.Once
	defaultBackendFunc func() Backend = func() Backend {
		panic("platform unsupported")
	}
	defaultBackend Backend
)

func DefaultBackend() Backend {
	defaultBackendOnce.Do(func() {
		defaultBackend = defaultBackendFunc()
	})
	return defaultBackend
}
