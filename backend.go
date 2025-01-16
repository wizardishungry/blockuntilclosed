package blockuntilclosed

import (
	"sync"
)

type Backend interface {
	Done(fd uintptr) (<-chan struct{}, error)
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
