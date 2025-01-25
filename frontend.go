package blockuntilclosed

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// Frontend is the interface for the end user to interact with the package.
// The user may also use package scope convenience methods that delegate to the default frontend.
type Frontend interface {
	Done(conn Conn) <-chan struct{}
	WithContext(ctx context.Context, conn Conn) context.Context
	SetLogger(logger *log.Logger)
	Close() error
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

var alwaysDone <-chan struct{} = func() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()

func (fe *frontend) Done(conn Conn) <-chan struct{} {
	return fe.WithContext(context.Background(), conn).Done()
}

func (fe *frontend) WithContext(ctx context.Context, conn Conn) context.Context {
	ctx, cancelCause := context.WithCancelCause(ctx)

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

func (fe *frontend) SetLogger(logger *log.Logger) {
	fe.logger = logger
}

func (fe *frontend) Close() error {
	return fe.backend.Close()
}
