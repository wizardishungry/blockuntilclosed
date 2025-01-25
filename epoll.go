//go:build linux

package blockuntilclosed

import (
	"errors"
	"fmt"
	"log"

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

// epoll is a MultiplexBackend that uses epoll(7) to block until a file descriptor is closed.
// Do not initialize this struct directly, use NewEpoll instead.
type epoll struct {
	logger *log.Logger
}

func NewEpoll() *MultiplexBackend {
	return NewMultiplexBackend(&epoll{})
}

func (ep *epoll) SetLogger(logger *log.Logger) {
	ep.logger = logger
}

func (ep *epoll) Start(cancelFD int) (_ int, finalErr error) {
	epfd, err := unix.EpollCreate1(0)
	if err != nil {
		return -1, fmt.Errorf("%w: %w", ErrEpollCtl, err)
	}

	if err := unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, cancelFD, &unix.EpollEvent{
		Events: unix.EPOLLIN |
			unix.EPOLLOUT |
			unix.EPOLLRDHUP |
			unix.EPOLLERR,
		// unix.EPOLLONESHOT,
		Fd: int32(cancelFD),
	}); err != nil {
		return -1, fmt.Errorf("%w: %w", ErrEpollCtl, err)
	}

	return epfd, nil
}

func (ep *epoll) Subscribe(pollfd, fd int) error {
	err := unix.EpollCtl(pollfd, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{
		Events: unix.EPOLLRDHUP | unix.EPOLLONESHOT | unix.EPOLLERR,
		Fd:     int32(fd),
	})
	return err
}

func (ep *epoll) Recv(pollFD int) (int, bool, error) {
	var events [1]unix.EpollEvent

	n, err := unix.EpollWait(pollFD, events[:], -1)
	if err != nil {
		return -1, false, fmt.Errorf("unix.EpollWait(): %w", err)
	}
	if n == 0 {
		ep.logger.Printf("unix.EpollWait(): no events")
		return -1, false, nil
	}
	ev := &events[0]
	ep.logger.Printf("unix.EpollWait(): got event %+v", ev)
	fd := int(ev.Fd)

	return fd, true, nil
}
