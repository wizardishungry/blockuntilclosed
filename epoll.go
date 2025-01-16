//go:build linux

package blockuntilclosed

import (
	"log"
	"os"

	"golang.org/x/sys/unix"
)

func init() {
	defaultBackendFunc = func() Backend {
		return NewEpoll()
	}
}

// KQueue is a Backend that uses kqueue(2) to block until a file descriptor is closed.
// Do not initialize this struct directly, use NewKQueue instead.
type Epoll struct {
	logger *log.Logger
}

func NewEpoll() *Epoll {
	logger := log.New(os.Stderr, "Epoll: ", log.LstdFlags)

	return &Epoll{
		logger: logger,
	}
}

func (ep *Epoll) SetLogger(logger *log.Logger) {
	ep.logger = logger
}

func (ep *Epoll) Close() error {
	return nil
}

// TODO read notes here https://stackoverflow.com/questions/70905227/epoll-does-not-signal-an-event-when-socket-is-close

func (ep *Epoll) Done(fd uintptr) <-chan struct{} {
	epollFD, err := unix.EpollCreate1(0)
	if err != nil {
		ep.logger.Fatalf("unix.EpollCreate1(): %v", err)
		return nil
	}

	if err := unix.EpollCtl(epollFD, unix.EPOLL_CTL_ADD, int(fd), &unix.EpollEvent{
		Events: unix.EPOLLIN | unix.EPOLLRDHUP | unix.EPOLLONESHOT,
		Fd:     int32(fd),
	}); err != nil {
		defer unix.Close(epollFD)
		ep.logger.Fatalf("unix.EpollCtl(): %v", err)
		return nil
	}

	c := make(chan struct{})
	go func() {
		defer unix.Close(epollFD)
		defer close(c)
		events := make([]unix.EpollEvent, 1)
		for {
			// TODO need to catch interrupted system call
			n, err := unix.EpollWait(epollFD, events, -1)
			if err != nil {
				ep.logger.Printf("unix.EpollWait(): %v", err)
				return
			}
			if n == 0 {
				ep.logger.Printf("unix.EpollWait(): no events")
				return
			}
			// ep.logger.Printf("unix.EpollWait(): %+v", events[0])
			return
		}
	}()

	return c
}
