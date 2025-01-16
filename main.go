package blockuntilclosed

import (
	"context"
	"net"
	"os"
	"syscall"
)

var _ syscall.Conn = (*os.File)(nil)
var _ syscall.Conn = (*net.TCPConn)(nil)

type Conn interface {
	syscall.Conn
	// net.Conn // TODO: this is should be constrained on mac because we don't know how to do this for os.File.
}

// Done blocks until a file descriptor is closed.
func Done(conn Conn) <-chan struct{} {
	return WithBackend(DefaultBackend()).Done(conn)
}

// WithContext returns a wrapped Context that is canceled when the file descriptor is closed.
func WithContext(ctx context.Context, conn Conn) context.Context {
	return WithBackend(DefaultBackend()).WithContext(ctx, conn)
}
