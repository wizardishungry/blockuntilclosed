//go:build freebsd || openbsd || netbsd || dragonfly || darwin

package blockuntilclosed

import (
	"fmt"
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
}

func NewKQueue() *KQueue {
	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		panic(fmt.Errorf("os.Pipe(): %w", err))
	}

	return &KQueue{
		pipeRead:  pipeRead,
		pipeWrite: pipeWrite,
	}
}

func (kq *KQueue) Close() error {
	kq.pipeRead.Close()
	kq.pipeWrite.Close()
	return nil
}

func (kq *KQueue) Done(fd uintptr) (<-chan struct{}, error) {
	var cancelFD uintptr
	if kq.pipeRead != nil {
		cancelFD = kq.pipeRead.Fd()
	}
	return darwin_kqueue(fd, cancelFD)
}

// cribbed from https://gist.github.com/juanqui/7564275
func darwin_kqueue(fd, cancelFD uintptr) (<-chan struct{}, error) {
	done := make(chan struct{})

	kqfd, err := unix.Kqueue() // TODO: this kqueue should be global to avoid creating it every time
	if err != nil {
		return nil, fmt.Errorf("unix.Kqueue(): %w", err)
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

		timeout := &unix.Timespec{
			Sec:  0,
			Nsec: 100000,
		}
		timeout = nil

		var eventsOut [1]unix.Kevent_t

		n, err := unix.Kevent(kqfd, eventsIn, eventsOut[:], timeout)
		if err != nil {
			panic(fmt.Errorf("unix.Kevent(): %w", err)) // TODO revise error handling
		}

		fmt.Println("got", n)
		gotEvent := eventsOut[0]

		fromSocket := uintptr(gotEvent.Ident) == fd

		// vanishedFlag := gotEvent.Flags&unix.EV_VANISHED != 0 // TODO do we need this?
		errorFlag := gotEvent.Flags&unix.EV_ERROR != 0
		eofFlag := gotEvent.Flags&unix.EV_EOF != 0

		fmt.Printf("gotEvent: %+v fromSocket=%+v\n", gotEvent, fromSocket)

		if eofFlag {
			fmt.Println("eof")
		}
		if errorFlag {
			fmt.Printf("gotEvent.Flags&unix.EV_ERROR != 0: %v\n", unix.Errno(gotEvent.Data))
		}
	}()

	return done, nil
}
