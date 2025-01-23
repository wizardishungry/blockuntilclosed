//go:build freebsd || openbsd || netbsd || dragonfly || darwin

package blockuntilclosed

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

func init() {
	defaultBackendFunc = func() Backend {
		return NewKQueue()
	}
}

// KQueue is a Backend that uses kqueue(2) to block until a file descriptor is closed.
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
type KQueue struct {
	pipeRead, pipeWrite *os.File
	logger              *log.Logger
	kqfd                int
	closeOnce           func() error
	allDone             chan struct{}
	m                   closeMap
}

// NewKQueue returns a new KQueue instance.
func NewKQueue() *KQueue {
	logger := log.New(os.Stderr, "KQueue: ", log.LstdFlags)

	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		logger.Fatalf("os.Pipe(): %v", err)
	}

	kq := &KQueue{
		pipeRead:  pipeRead,
		pipeWrite: pipeWrite,
		logger:    logger,
		allDone:   make(chan struct{}),
		closeOnce: nil,
		m:         closeMap{},
	}
	kq.closeOnce = sync.OnceValue(kq.close)

	if err := kq.startKQueue(); err != nil {
		logger.Fatalf("kq.startKQueue(): %v", err)
	}

	return kq
}

func (kq *KQueue) SetLogger(logger *log.Logger) {
	kq.logger = logger
}

func (kq *KQueue) getMap() *closeMap {
	return &kq.m
}

func (kq *KQueue) close() error {
	err := errors.Join(
		kq.pipeRead.Close(),
		kq.pipeWrite.Close(),
	)
	<-kq.allDone
	count := kq.m.Drain()
	kq.logger.Printf("Drain(): %d", count)
	return err
}

func (kq *KQueue) Close() error {
	return kq.closeOnce()
}

func (kq *KQueue) Done(fd int) <-chan struct{} {
	select {
	case <-kq.allDone:
		return nil
	default:
	}

	loaded, payload := kq.m.Add(fd)
	if payload == nil {
		kq.logger.Print("nil payload; this is a problem")
		return nil
	}

	if loaded && payload.c == nil {
		kq.logger.Print("loaded and nil; this is a problem")
		return nil
	}

	if loaded {
		// Already added
		kq.logger.Print("Already added")
		return payload.c
	}

	eventsIn := [...]unix.Kevent_t{
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

	var eventsOut [1]unix.Kevent_t

RETRY:
	_, err := unix.Kevent(kq.kqfd, eventsIn[:], eventsOut[:], nil)
	if errors.Is(err, unix.EINTR) {
		kq.logger.Print("Done unix.Kevent EINTR")
		goto RETRY
	} else if err != nil {
		kq.logger.Printf("Done unix.Kevent(): %v", err)
		return nil
	}

	kq.logger.Print("Done success")

	return payload.c
}

func (kq *KQueue) startKQueue() error {
RETRY_Kqueue:
	kqfd, err := unix.Kqueue()
	if errors.Is(err, unix.EINTR) {
		kq.logger.Print("startKQueue unix.Kqueue EINTR")
		goto RETRY_Kqueue
	} else if err != nil {
		return fmt.Errorf("unix.Kqueue(): %w", err)
	}

	kq.kqfd = kqfd
	cancelFD := kq.pipeRead.Fd()

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

RETRY_Kevent:
	_, err = unix.Kevent(kqfd, eventsIn[:], eventsOut[:], nil)
	if errors.Is(err, unix.EINTR) {
		kq.logger.Print("startup unix.Kevent EINTR")
		goto RETRY_Kevent
	} else if err != nil {
		return fmt.Errorf("startup unix.Kevent(): %w", err)
	}

	go kq.worker(cancelFD)

	return nil
}

func (kq *KQueue) worker(cancelFD uintptr) {
	defer close(kq.allDone)
	defer func() {
		err := unix.Close(kq.kqfd)
		if err != nil {
			kq.logger.Printf("worker unix.Close(): %v", err)
		}
	}()

	var (
		eventsOut [1]unix.Kevent_t
	)
	for {
	RETRY:
		n, err := unix.Kevent(kq.kqfd, nil, eventsOut[:], nil)
		if errors.Is(err, unix.EINTR) {
			kq.logger.Print("pool kqueue EINTR")
			goto RETRY
		} else if err != nil {
			kq.logger.Printf("poll unix.Kevent(): %v", err)
			return
		}

		kq.logger.Print("kqueue events received ", n)

		ev := &eventsOut[0]

		if ev.Ident == uint64(cancelFD) {
			kq.logger.Print("cancelFD event")
			return
		}

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

		if errorFlag {
			kq.logger.Print("errorFlag, not closing")
			continue
		}

		if ev.Filter != unix.EVFILT_EXCEPT {
			kq.logger.Print("not EVFILT_EXCEPT, not closing")
			continue
		}

		closed := kq.m.Close(int(ev.Ident))
		kq.logger.Print("Close success=", closed)
	}
}
