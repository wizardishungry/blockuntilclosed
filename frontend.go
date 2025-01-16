package blockuntilclosed

import (
	"context"
	"errors"
	"log"
	"sync"
)

type Frontend interface {
	Done(conn Conn) <-chan struct{}
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
}

func newFrontend(b Backend) *frontend {
	return &frontend{
		backend: b,
	}
}

func (fe *frontend) Done(conn Conn) <-chan struct{} {
	sconn, err := conn.SyscallConn()

	if err != nil {
		log.Printf("conn.SyscallConn(): %v", err)
		return nil
	}
	var (
		done <-chan struct{}
	)
	if err := sconn.Control(func(fd uintptr) {
		done = fe.backend.Done(fd)
	}); err != nil {
		log.Printf("sconn.Control(): %v", err)
	}

	return done
}

var ErrConnClosed = errors.New("conn closed")

func (fe *frontend) WithContext(ctx context.Context, conn Conn) context.Context {
	ctx, cancelCause := context.WithCancelCause(ctx)
	go func() {
		defer cancelCause(nil)
		done := fe.Done(conn)
		select {
		case <-done:
			cancelCause(ErrConnClosed)
		case <-ctx.Done():
			cancelCause(ctx.Err())
		}
	}()

	return ctx
}
