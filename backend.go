package blockuntilclosed

import (
	"log"
	"sync"
	"syscall"
)

// Backend is the interface for the platform-specific implementation of the package.
type Backend interface {
	Done(sconn syscall.RawConn, fd uintptr) <-chan struct{}
	SetLogger(logger *log.Logger)
	getMap() *closeMap
}

var (
	defaultBackendFunc func() Backend = func() Backend {
		log.Fatal("platform not supported")
		return nil
	}
	defaultBackendOnceFunc = sync.OnceValue(func() Backend {
		return NewDefaultBackend()
	})
)

// DefaultBackend retrieves a singleton instance of the default backend for the current platform.
func DefaultBackend() Backend {
	return defaultBackendOnceFunc()
}

// NewDefaultBackend returns a new instance of the default backend for the current platform.
func NewDefaultBackend() Backend {
	return defaultBackendFunc()
}
