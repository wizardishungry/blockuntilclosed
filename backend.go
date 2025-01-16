package blockuntilclosed

import (
	"log"
	"sync"
)

type Backend interface {
	Done(fd uintptr) <-chan struct{}
}

var (
	defaultBackendOnce sync.Once
	defaultBackendFunc func() Backend = func() Backend {
		log.Fatal("platform not supported")
		return nil
	}
	defaultBackend Backend
)

func DefaultBackend() Backend {
	defaultBackendOnce.Do(func() {
		defaultBackend = defaultBackendFunc()
	})
	return defaultBackend
}
