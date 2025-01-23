package blockuntilclosed

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
)

var _ syscall.Conn = (*os.File)(nil)
var _ syscall.Conn = (*net.TCPConn)(nil)

// package scope errors may extracted from canceled contexts using [context.Cause].
var (
	ErrConnClosed = errors.New("conn closed")
)

type Conn interface {
	syscall.Conn
	// net.Conn // TODO: This should be constrained on mac because we don't know how to do this for os.File.
}

// Done blocks until a file descriptor is closed.
func Done(conn Conn) <-chan struct{} {
	return DefaultFrontend().Done(conn)
}

// WithContext returns a wrapped Context that is canceled when the file descriptor is closed.
func WithContext(ctx context.Context, conn Conn) context.Context {
	return DefaultFrontend().WithContext(ctx, conn)
}
