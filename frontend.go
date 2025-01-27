package blockuntilclosed

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// FrontendSingleton is the interface for the end user to interact with the default frontend.
// The user may also use package scope convenience methods that delegate to the default frontend.
type FrontendSingleton interface {
	Done(conn Conn) <-chan struct{}
	WithContext(ctx context.Context, conn Conn) context.Context
}

var (
	defaultFrontendFunc = sync.OnceValue(func() FrontendSingleton {
		return NewFrontend(DefaultBackend())
	})
)

// DefaultFrontend retrieves a singleton instance of the default frontend for the current platform.
// The default frontend uses the default backend for the current platform.
// It is created on first use.
func DefaultFrontend() FrontendSingleton {
	return defaultFrontendFunc()
}

// NewFrontend returns a new instance of the frontend with the specified backend.
func NewFrontend(b Backend, opts ...FrontendOption) *Frontend {
	logger := log.New(os.Stderr, "blockuntilclosed: ", log.LstdFlags)

	fe := &Frontend{
		backend: b,
		logger:  logger,
	}

	for _, opt := range opts {
		opt(fe)
	}
	return fe
}

type FrontendOption func(*Frontend)

// Frontend is a uniform interface for OS-specific backends.
type Frontend struct {
	backend Backend
	logger  *log.Logger
}

func (fe *Frontend) Done(conn Conn) <-chan struct{} {
	return fe.WithContext(context.Background(), conn).Done()
}

func (fe *Frontend) WithContext(ctx context.Context, conn Conn) context.Context {
	ctx, cancelCause := context.WithCancelCause(ctx)

	// Maybe some platforms would have an approach that doesn't require a syscall?

	sconn, err := conn.SyscallConn()
	if err != nil {
		cancelCause(fmt.Errorf("%w: %w", ErrSyscallConn, err))
		return ctx
	}

	if err := sconn.Control(func(fd uintptr) {
		newFD, err := unix.Dup(int(fd))
		if err != nil {
			cancelCause(fmt.Errorf("%w: %w", ErrDup, err))
			return
		}
		fe.logger.Printf("newFD: %d->%d", fd, newFD)
		// TODO: at this point we could call WithFD asynchronously and early return the context.
		fe.backend.WithFD(ctx, cancelCause, newFD)
	}); err != nil {
		cancelCause(fmt.Errorf("%w: %w", ErrControl, err))
	}

	return ctx
}

func (fe *Frontend) SetLogger(logger *log.Logger) {
	fe.logger = logger
}

func (fe *Frontend) Close() error {
	return fe.backend.Close()
}
