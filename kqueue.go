//go:build freebsd || openbsd || netbsd || dragonfly || darwin

package blockuntilclosed

import (
	"errors"
	"fmt"
	"log"

	"golang.org/x/sys/unix"
)

func init() {
	defaultBackendFunc = func() Backend {
		return NewKQueue()
	}
}

var (
	ErrKeventAdd = errors.New("kevent add")
)

// kqueue is a MultiplexBackend that uses kqueue(2) to block until a file descriptor is closed.
// Do not initialize this struct directly, use NewKQueue instead.
//
// Cribbed from
//   - https://gist.github.com/juanqui/7564275
//   - https://github.com/apple/darwin-xnu/blob/main/bsd/sys/event.h
//   - https://github.com/fsnotify/fsnotify/blob/main/backend_kqueue.go
//
// For os.File support, FreeBSD, etc. supports  the following filter types:
//
//	   NOTE_CLOSE		A  file	descriptor referencing
//				the   monitored	  file,	   was
//				closed.	  The  closed file de-
//				scriptor did  not  have	 write
//				access.
//	   NOTE_CLOSE_WRITE	A  file	descriptor referencing
//				the   monitored	  file,	   was
//				closed.	  The  closed file de-
//				scriptor had write access.

// NewKQueue creates a new instance of KQueue.
func NewKQueue() *MultiplexBackend {
	return NewMultiplexBackend(&kqueue{})
}

type kqueue struct {
	logger *log.Logger
}

func (kq *kqueue) SetLogger(logger *log.Logger) {
	kq.logger = logger
}

func (kq *kqueue) Subscribe(pollfd, fd int) error {
	var (
		eventsIn = [...]unix.Kevent_t{
			{ // EVFILT_EXCEPT is used to detect when the socket disconnects.
				Ident:  uint64(fd),
				Filter: unix.EVFILT_EXCEPT,
				Flags: unix.EV_ADD |
					unix.EV_ENABLE |
					unix.EV_ONESHOT |
					unix.EV_DISPATCH2 |
					// unix.EV_DISPATCH |
					unix.EV_VANISHED |
					unix.EV_RECEIPT,
				Fflags: unix.NOTE_NONE,
				Data:   0,
				Udata:  nil,
			},
		}
		eventsOut [1]unix.Kevent_t
	)

	_, err := unix.Kevent(pollfd, eventsIn[:], eventsOut[:], nil)
	return err
}

func (kq *kqueue) Start(cancelFD int) (int, error) {
	kqfd, err := unix.Kqueue()
	if err != nil {
		return -1, fmt.Errorf("unix.Kqueue(): %w", err)
	}

	eventsIn := [...]unix.Kevent_t{{
		Ident:  uint64(cancelFD),
		Filter: unix.EVFILT_READ,
		Flags: unix.EV_ADD | // add the event
			unix.EV_ENABLE | // enable the event
			unix.EV_ONESHOT | // deliver this event only once
			unix.EV_DISPATCH2 | // prereq for EV_VANISHED?
			unix.EV_VANISHED | // I believe necessary if the pipe is closed
			unix.EV_RECEIPT, // do not receive only add
		Fflags: unix.NOTE_NONE,
		Data:   0,
		Udata:  nil,
	}}

	var eventsOut [1]unix.Kevent_t

	_, err = unix.Kevent(kqfd, eventsIn[:], eventsOut[:], nil)
	if err != nil {
		return kqfd, fmt.Errorf("startup unix.Kevent(): %w", err)
	}

	return kqfd, nil
}

func (kq *kqueue) Recv(pollFD int) (fd int, isClose bool, _ error) {
	var (
		eventsOut [1]unix.Kevent_t
	)
	n, err := unix.Kevent(pollFD, nil, eventsOut[:], nil)
	if err != nil {
		return -1, false, fmt.Errorf("poll unix.Kevent(): %w", err)
	}

	kq.logger.Print("kqueue events received ", n)

	ev := &eventsOut[0]

	errorFlag := ev.Flags&unix.EV_ERROR != 0
	eofFlag := ev.Flags&unix.EV_EOF != 0

	var filterName string
	switch ev.Filter {
	case unix.EVFILT_READ:
		filterName = "EVFILT_READ"
	case unix.EVFILT_WRITE:
		filterName = "EVFILT_WRITE"
	case unix.EVFILT_AIO:
		filterName = "EVFILT_AIO"
	case unix.EVFILT_VNODE:
		filterName = "EVFILT_VNODE"
	case unix.EVFILT_PROC:
		filterName = "EVFILT_PROC"
	case unix.EVFILT_SIGNAL:
		filterName = "EVFILT_SIGNAL"
	case unix.EVFILT_TIMER:
		filterName = "EVFILT_TIMER"
	case unix.EVFILT_EXCEPT:
		filterName = "EVFILT_EXCEPT"
	default:
		filterName = fmt.Sprintf("unknown filter %d", ev.Filter)
	}

	errno := unix.Errno(ev.Fflags)
	kq.logger.Printf("event: %+v errorFlag=%v eofFlag=%v filterName=%v errno=%v\n", ev, errorFlag, eofFlag, filterName, errno)

	return int(ev.Ident), true, nil
}
