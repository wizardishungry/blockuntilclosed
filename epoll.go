//go:build linux

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

func init() {
	defaultBackendFunc = func() Backend {
		return NewEpoll()
	}
}

var (
	ErrEpollCtl = errors.New("epoll ctl")
)

// Epoll is a Backend that uses epoll(7) to block until a file descriptor is closed.
// Do not initialize this struct directly, use NewEpoll instead.
type Epoll struct {
	logger              *log.Logger
	m                   closeMap
	epollFD             int
	allDone             chan struct{}
	pipeRead, pipeWrite *os.File
	closeOnce           func() error
}

func NewEpoll() *Epoll {
	logger := log.New(os.Stderr, "Epoll: ", log.LstdFlags)

	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		logger.Fatalf("os.Pipe(): %v", err)
	}

RETRY:
	epollFD, err := unix.EpollCreate1(0)
	if errors.Is(err, unix.EINTR) {
		logger.Print("NewEpoll unix.EpollCreate1 EINTR")
		goto RETRY
	} else if err != nil {
		logger.Fatalf("unix.EpollCreate1(): %v", err)
		return nil
	}

	ep := &Epoll{
		m:         closeMap{},
		logger:    logger,
		epollFD:   epollFD,
		pipeRead:  pipeRead,
		pipeWrite: pipeWrite,
		allDone:   make(chan struct{}),
	}
	ep.closeOnce = sync.OnceValue(ep.close)

	pipeFD := int(pipeRead.Fd())
	err = ep.registerPipe(pipeFD)
	if err != nil {
		logger.Fatalf("registerPipe(): %v", err)
		return nil
	}

	go ep.worker(pipeFD)

	return ep
}

func (ep *Epoll) SetLogger(logger *log.Logger) {
	ep.logger = logger
}

func (ep *Epoll) getMap() *closeMap {
	return &ep.m
}

func (ep *Epoll) Close() error {
	ep.logger.Print("Close()")
	return ep.closeOnce()
}

func (ep *Epoll) close() error {
	ep.pipeWrite.Write([]byte{0})
	ep.logger.Print("awaiting allDone")
	<-ep.allDone
	count := ep.m.Drain()
	ep.logger.Printf("Drain(): %d", count)
	return nil
}

func (ep *Epoll) registerPipe(cancelFD int) error {
RETRY:
	if err := unix.EpollCtl(ep.epollFD, unix.EPOLL_CTL_ADD, cancelFD, &unix.EpollEvent{
		Events: unix.EPOLLIN |
			unix.EPOLLOUT |
			unix.EPOLLRDHUP |
			unix.EPOLLERR,
		// unix.EPOLLONESHOT,
		Fd: int32(cancelFD),
	}); errors.Is(err, unix.EINTR) {
		ep.logger.Print("registerPipe unix.EpollCtl EINTR")
		goto RETRY
	} else if err != nil {
		return fmt.Errorf("%w: %w", ErrEpollCtl, err)
	}
	return nil
}

func (ep *Epoll) worker(cancelFD int) {
	defer ep.pipeRead.Close()
	defer ep.pipeWrite.Close()
	defer unix.Close(ep.epollFD)
	defer close(ep.allDone)
	var events [1]unix.EpollEvent

	ep.logger.Printf("worker started on %d with cancelFD %d", ep.epollFD, cancelFD)

	for {
	RETRY:
		n, err := unix.EpollWait(ep.epollFD, events[:], -1)
		if errors.Is(err, unix.EINTR) {
			ep.logger.Print("unix.EpollWait EINTR")
			goto RETRY
		}
		if err != nil {
			ep.logger.Printf("unix.EpollWait(): %v", err)
			return
		}
		if n == 0 {
			ep.logger.Printf("unix.EpollWait(): no events")
			return
		}
		ev := &events[0]
		ep.logger.Printf("unix.EpollWait(): got event %+v", ev)
		fd := int(ev.Fd)

		if fd == cancelFD {
			ep.logger.Print("cancelFD triggered")
			return
		}

		closed := ep.m.Close(fd, ErrConnClosed)
		ep.logger.Printf("Close(%d): %v", fd, closed)
	}
}

func (ep *Epoll) WithFD(ctx context.Context, cancelCause context.CancelCauseFunc, fd int) {
	select {
	case <-ep.allDone:
		cancelCause(ErrBackendClosed)
		return
	default:
	}

	loaded, payload := ep.m.Add(fd, ctx, cancelCause)
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
		ep.m.Close(fd, err)
	}

RETRY:
	select {
	case <-ep.allDone:
		cancelCause(ErrBackendClosed)
		return
	case <-ctx.Done():
		return
	default:
	}

	if err := unix.EpollCtl(ep.epollFD, unix.EPOLL_CTL_ADD, int(fd), &unix.EpollEvent{
		Events: unix.EPOLLIN | unix.EPOLLRDHUP | unix.EPOLLONESHOT | unix.EPOLLERR,
		Fd:     int32(fd),
	}); errors.Is(err, unix.EINTR) {
		ep.logger.Print("Done unix.EpollCtl EINTR")
		goto RETRY
	} else if err != nil {
		cancelCause(fmt.Errorf("%w: %w", ErrEpollCtl, err))
		return
	}

	ep.logger.Printf("Done(): added %d", fd)
}
