package blockuntilclosed

import (
	"context"
	"log"
	"os"
	"sync"
)

// Frontend is the interface for the end user to interact with the package.
// The user may also use package scope convenience methods that delegate to the default frontend.
type Frontend interface {
	Done(conn Conn) <-chan struct{}
	WithContext(ctx context.Context, conn Conn) context.Context
	SetLogger(logger *log.Logger)
}

var (
	defaultFrontendFunc = sync.OnceValue(func() Frontend {
		return WithBackend(DefaultBackend())
	})
)

// DefaultFrontend retrieves a singleton instance of the default frontend for the current platform.
func DefaultFrontend() Frontend {
	return defaultFrontendFunc()
}

// WithBackend returns a new instance of the frontend with the specified backend.
func WithBackend(b Backend) Frontend {
	return newFrontend(b)
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

func (fe *frontend) WithContext(ctx context.Context, conn Conn) context.Context {
	ctx, cancelCause := context.WithCancelCause(ctx)
	go func() {
		defer cancelCause(nil)
		done := fe.Done(conn)
		select {
		case <-done:
			cancelCause(ErrConnClosed)
		case <-ctx.Done():
			cancelCause(context.Cause(ctx))
		}
	}()

	return ctx
}

func (fe *frontend) SetLogger(logger *log.Logger) {
	fe.logger = logger
}
