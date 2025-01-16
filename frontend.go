package blockuntilclosed

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

type Frontend interface {
	Done(conn Conn) (<-chan struct{}, error)
	WithContext(ctx context.Context, conn Conn) context.Context
}

var (
	frontends      = make(map[Backend]Frontend)
	frontendsMutex sync.Mutex
)

func WithBackend(b Backend) Frontend {
	frontendsMutex.Lock()
	defer frontendsMutex.Unlock()
	fe := frontends[b]
	if fe == nil {
		fe = newFrontend(b)
		frontends[b] = fe
	}
	return fe
}

type frontend struct {
	backend Backend
	cache   map[Conn]<-chan struct{}
}

func newFrontend(b Backend) *frontend {
	return &frontend{
		backend: b,
		cache:   make(map[Conn]<-chan struct{}),
	}
}

func (fe *frontend) Done(conn Conn) (<-chan struct{}, error) {
	sconn, err := conn.SyscallConn()

	if err != nil {
		return nil, fmt.Errorf("conn.SyscallConn(): %w", err)
	}
	var (
		done  <-chan struct{}
		myErr error
		// TODO: checks for OpError should be in IsClosed as well
		opError *net.OpError
	)
	if err := sconn.Control(func(fd uintptr) {
		done, myErr = fe.backend.Done(fd)
	}); errors.As(err, &opError) {
		if opError.Op == "raw-control" {
			return nil, nil // trying to call Control on a closed socket indicates that the socket is closed!
		}
	} else if err != nil {
		return nil, fmt.Errorf("sconn.Control(): %w", err)
	}
	return done, myErr
}

func (fe *frontend) WithContext(ctx context.Context, conn Conn) context.Context {
	ctx, cancelCause := context.WithCancelCause(ctx)
	go func() {
		defer cancelCause(nil)
		done, err := fe.Done(conn)
		if err != nil {
			cancelCause(err)
		}
		select {
		case <-done:
			cancelCause(nil)
		case <-ctx.Done():
			cancelCause(ctx.Err())
		}
	}()

	return ctx
}
