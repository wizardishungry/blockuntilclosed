package blockuntilclosed

import (
	"log"
	"sync"
	"syscall"
)

type Backend interface {
	Done(sconn syscall.RawConn, fd uintptr) <-chan struct{}
	SetLogger(logger *log.Logger)
	getMap() *closeMap
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
		defaultBackend = NewDefaultBackend()
	})
	return defaultBackend
}

func NewDefaultBackend() Backend {
	return defaultBackendFunc()
}
