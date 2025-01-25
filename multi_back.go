package blockuntilclosed

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// NewMultiplexBackend returns a new MultiplexBackend instance.
func NewMultiplexBackend(platform MultiplexPlatform) *MultiplexBackend {
	logger := log.New(os.Stderr, fmt.Sprintf("%T: ", platform),
		log.LstdFlags|log.Lshortfile|log.Lmicroseconds,
	)
	platform.SetLogger(logger)

	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		logger.Fatalf("os.Pipe(): %v", err)
	}

	be := &MultiplexBackend{
		pipeRead:  pipeRead,
		pipeWrite: pipeWrite,
		logger:    logger,
		allDone:   make(chan struct{}),
		closeOnce: nil,
		m:         closeMap{},
		platform:  platform,
	}

	be.closeOnce = sync.OnceValue(be.close)
	cancelFD := int(pipeRead.Fd())

RETRY:
	be.pollfd, err = be.platform.Start(cancelFD)
	if errors.Is(err, unix.EINTR) {
		if be.pollfd > 0 {
			unix.Close(be.pollfd)
		}
		logger.Print("Start() unix.Kqueue EINTR")
		goto RETRY
	} else if err != nil {
		logger.Fatalf("Start(): %v", err)
	}

	go be.worker(cancelFD)

	return be
}

// MultiplexBackend is a Backend that uses kqueue(2) or epoll(7) to block until a file descriptor is closed.
type MultiplexBackend struct {
	platform            MultiplexPlatform
	pipeRead, pipeWrite *os.File
	logger              *log.Logger
	pollfd              int
	closeOnce           func() error
	allDone             chan struct{}
	m                   closeMap
}

func (be *MultiplexBackend) WithFD(ctx context.Context, cancelCause context.CancelCauseFunc, fd int) {
	select {
	case <-be.allDone:
		cancelCause(ErrBackendClosed)
		return
	default:
	}

	loaded, payload := be.m.Add(fd, ctx, cancelCause)
	if payload == nil {
		cancelCause(ErrBackendMapAddNil)
		return
	}

	if loaded && payload.cancelCause == nil {
		cancelCause(ErrBackendMapAddNoCancel)
		return
	}

	if loaded {
		cancelCause(ErrBackEndMapAlreadyAdded)
		return
	}

	// Replace context cancel with a call to Close that also.
	cancelCause = func(err error) {
		be.m.Close(fd, err)
	}

RETRY:
	select {
	case <-be.allDone:
		cancelCause(ErrBackendClosed)
	case <-ctx.Done():
		return
	default:
	}

	err := be.platform.Subscribe(be.pollfd, fd)
	if errors.Is(err, unix.EINTR) {
		be.logger.Print("Done unix.Kevent EINTR")
		goto RETRY
	} else if err != nil {
		cancelCause(err)
		return
	}

	be.logger.Print("Done success")
}

func (be *MultiplexBackend) SetLogger(logger *log.Logger) {
	be.logger = logger
	be.platform.SetLogger(logger)
}

func (be *MultiplexBackend) Close() error {
	return be.closeOnce()
}

func (be *MultiplexBackend) close() error {
	err := errors.Join(
		be.pipeRead.Close(),
		be.pipeWrite.Close(),
	)
	<-be.allDone
	count := be.m.Drain()
	be.logger.Printf("Drain(): %d", count)
	return err
}

func (be *MultiplexBackend) worker(cancelFD int) {
	defer close(be.allDone)
	defer func() {
		err := unix.Close(be.pollfd)
		if err != nil {
			be.logger.Printf("worker unix.Close(): %v", err)
		}
	}()

	for {
	RETRY:
		fd, isClose, err := be.platform.Recv(be.pollfd)
		if errors.Is(err, unix.EINTR) {
			be.logger.Print("pool kqueue EINTR")
			goto RETRY
		}

		if fd == cancelFD {
			be.logger.Print("cancelFD event")
			return
		}

		if isClose {
			closed := be.m.Close(fd, ErrConnClosed)
			be.logger.Printf("closed %d %v", fd, closed)
		} else {
			be.logger.Printf("not closing? %d", fd)
		}
	}
}
