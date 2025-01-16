package blockuntilclosed

import (
	"context"
	"errors"
	"log"
	"os"
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
	logger  *log.Logger
}

func newFrontend(b Backend) *frontend {
	logger := log.New(os.Stderr, "blockuntilclosed: ", log.LstdFlags)

	return &frontend{
		backend: b,
		logger:  logger,
	}
}

func (fe *frontend) Done(conn Conn) <-chan struct{} {
	sconn, err := conn.SyscallConn()

	if err != nil {
		fe.logger.Printf("conn.SyscallConn(): %v", err)
		return nil
	}
	var (
		done <-chan struct{}
	)
	if err := sconn.Control(func(fd uintptr) {
		done = fe.backend.Done(sconn, fd)
	}); err != nil {
		fe.logger.Printf("sconn.Control(): %v", err)
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
