//go:build freebsd || openbsd || netbsd || dragonfly || darwin

package blockuntilclosed

import (
	"log"
	"os"

	"golang.org/x/sys/unix"
)

func init() {
	defaultBackendFunc = func() Backend {
		return NewKQueue()
	}
}

// KQueue is a Backend that uses kqueue(2) to block until a file descriptor is closed.
// Do not initialize this struct directly, use NewKQueue instead.
type KQueue struct {
	pipeRead, pipeWrite *os.File
	logger              *log.Logger
}

func NewKQueue() *KQueue {
	logger := log.New(os.Stderr, "KQueue: ", log.LstdFlags)

	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		logger.Fatalf("os.Pipe(): %v", err)
	}

	return &KQueue{
		pipeRead:  pipeRead,
		pipeWrite: pipeWrite,
		logger:    logger,
	}
}

func (kq *KQueue) Close() error {
	kq.pipeRead.Close()
	kq.pipeWrite.Close()
	return nil
}

func (kq *KQueue) Done(fd uintptr) <-chan struct{} {
	var cancelFD uintptr
	if kq.pipeRead != nil {
		cancelFD = kq.pipeRead.Fd()
	}
	return kq.darwin_kqueue(fd, cancelFD)
}

// cribbed from https://gist.github.com/juanqui/7564275
func (kq *KQueue) darwin_kqueue(fd, cancelFD uintptr) <-chan struct{} {
	done := make(chan struct{})

	// TODO: this kqueue could be global to avoid creating it every time
	// I tried this but it was late at night.
	kqfd, err := unix.Kqueue()
	if err != nil {
		kq.logger.Printf("unix.Kqueue(): %v", err)
		return nil
	}

	go func() {
		defer close(done)
		defer unix.Close(kqfd)

		/*
			FreeBSD  supports  the following filter types:

						   NOTE_CLOSE		A  file	descriptor referencing
									the   monitored	  file,	   was
									closed.	  The  closed file de-
									scriptor did  not  have	 write
									access.

						   NOTE_CLOSE_WRITE	A  file	descriptor referencing
									the   monitored	  file,	   was
									closed.	  The  closed file de-
									scriptor had write access.

		*/

		// https://github.com/apple/darwin-xnu/blob/main/bsd/sys/event.h
		// https://github.com/fsnotify/fsnotify/blob/main/backend_kqueue.go

		// This only works for sockets (and fifos?). It doesn't work for files.
		ev1 := unix.Kevent_t{
			Ident:  uint64(fd),
			Filter: unix.EVFILT_EXCEPT,
			Flags:  unix.EV_ADD | unix.EV_ENABLE | unix.EV_ONESHOT | unix.EV_DISPATCH2 | unix.EV_VANISHED,
			Fflags: unix.NOTE_NONE,
			Data:   0,
			Udata:  nil,
		}

		eventsIn := make([]unix.Kevent_t, 0, 2)
		eventsIn = append(eventsIn, ev1)

		if cancelFD > 0 {
			ev2 := unix.Kevent_t{
				Ident:  uint64(cancelFD),
				Filter: unix.EVFILT_READ,
				Flags:  unix.EV_ADD | unix.EV_ENABLE | unix.EV_ONESHOT | unix.EV_DISPATCH2 | unix.EV_VANISHED,
				Fflags: unix.NOTE_NONE,
				Data:   0,
				Udata:  nil,
			}
			eventsIn = append(eventsIn, ev2)
		}

		var eventsOut [1]unix.Kevent_t

		n, err := unix.Kevent(kqfd, eventsIn, eventsOut[:], nil)
		if err != nil {
			kq.logger.Printf("unix.Kevent(): %v", err)
			return
		}

		kq.logger.Print("kqueue events received", n)
		gotEvent := eventsOut[0]

		fromSocket := uintptr(gotEvent.Ident) == fd

		// vanishedFlag := gotEvent.Flags&unix.EV_VANISHED != 0 // TODO do we need this?
		errorFlag := gotEvent.Flags&unix.EV_ERROR != 0
		eofFlag := gotEvent.Flags&unix.EV_EOF != 0

		kq.logger.Printf("gotEvent: %+v fromSocket=%+v\n", gotEvent, fromSocket)

		if eofFlag {
			kq.logger.Print("got eof")
		}
		if errorFlag {
			kq.logger.Printf("gotEvent.Flags&unix.EV_ERROR != 0: %v\n", unix.Errno(gotEvent.Data))
		}
	}()

	return done
}
