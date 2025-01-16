package blockuntilclosed

import (
	"context"
	"errors"
	"fmt"
	"net"
)

type Frontend interface {
	Block(ctx context.Context, conn Conn) error
	IsClosed(conn Conn) (bool, error)
	WithContext(ctx context.Context, conn Conn) context.Context
}

func WithBackend(b Backend) Frontend {
	return &frontend{backend: b}
}

type frontend struct {
	backend Backend
}

func (fe *frontend) Block(ctx context.Context, conn Conn) error {
	sconn, err := conn.SyscallConn()

	if err != nil {
		return fmt.Errorf("conn.SyscallConn(): %w", err)
	}
	var (
		myErr error
		// TODO: checks for OpError should be in IsClosed as well
		opError *net.OpError
	)
	if err := sconn.Control(func(fd uintptr) {
		myErr = fe.backend.Block(ctx, fd)
	}); errors.As(err, &opError) {
		if opError.Op == "raw-control" {
			return nil // trying to call Control on a closed socket indicates that the socket is closed!
		}
	} else if err != nil {
		return fmt.Errorf("sconn.Control(): %w", err)
	}
	return myErr
}

func (fe *frontend) IsClosed(conn Conn) (bool, error) {
	sconn, err := conn.SyscallConn()
	if err != nil {
		return false, fmt.Errorf("conn.SyscallConn(): %w", err)
	}
	var (
		isClosed bool
		myErr    error
	)
	if err := sconn.Control(func(fd uintptr) {
		isClosed, myErr = fe.backend.IsClosed(fd)
	}); err != nil {
		return false, fmt.Errorf("sconn.Control(): %w", err)
	}
	fmt.Println("isClosed", isClosed, myErr)
	return isClosed, myErr
}

func (fe *frontend) WithContext(ctx context.Context, conn Conn) context.Context {
	ctx, cancelCause := context.WithCancelCause(ctx)
	go func() {
		defer cancelCause(nil)
		err := fe.Block(ctx, conn)
		cancelCause(err)
	}()

	return ctx
}
