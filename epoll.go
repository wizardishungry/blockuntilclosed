//go:build linux

package blockuntilclosed

import (
	"errors"
	"log"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func init() {
	defaultBackendFunc = func() Backend {
		return NewEpoll()
	}
}

// KQueue is a Backend that uses kqueue(2) to block until a file descriptor is closed.
// Do not initialize this struct directly, use NewEpoll instead.
// TODO read notes here https://stackoverflow.com/questions/70905227/epoll-does-not-signal-an-event-when-socket-is-close
type Epoll struct {
	logger  *log.Logger
	m       closeMap
	epollFD int
	allDone chan struct{}
}

func NewEpoll() *Epoll {
	logger := log.New(os.Stderr, "Epoll: ", log.LstdFlags)

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
		m:       closeMap{},
		logger:  logger,
		epollFD: epollFD,
		allDone: make(chan struct{}),
	}

	go ep.worker()

	return ep
}

func (ep *Epoll) SetLogger(logger *log.Logger) {
	ep.logger = logger
}

func (ep *Epoll) Close() error {
	return nil
}

func (ep *Epoll) worker() {
	defer unix.Close(ep.epollFD)
	defer close(ep.allDone)
	var events [1]unix.EpollEvent
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
		ep.logger.Printf("unix.EpollWait(): %+v", events[0])
		fd := uintptr(events[0].Fd)
		closed := ep.m.Close(fd)
		ep.logger.Printf("Close(): %v", closed)
	}
}

func (ep *Epoll) Done(sconn syscall.RawConn, fd uintptr) <-chan struct{} {
	select {
	case <-ep.allDone:
		return nil
	default:
	}

	loaded, payload := ep.m.Add(fd, sconn)
	if payload == nil {
		ep.logger.Print("nil payload; this is a problem")
		return nil
	}

	if loaded && payload.c == nil {
		ep.logger.Print("loaded and nil; this is a problem")
		return nil
	}

	if loaded {
		// Already added
		return payload.c
	}

RETRY:
	if err := unix.EpollCtl(ep.epollFD, unix.EPOLL_CTL_ADD, int(fd), &unix.EpollEvent{
		Events: unix.EPOLLIN | unix.EPOLLRDHUP | unix.EPOLLONESHOT,
		Fd:     int32(fd),
	}); errors.Is(err, unix.EINTR) {
		ep.logger.Print("Done unix.EpollCtl EINTR")
		goto RETRY
	} else if err != nil {
		ep.logger.Printf("unix.EpollCtl(): %v", err)
		return nil
	}

	return payload.c
}
