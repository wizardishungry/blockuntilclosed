//go:build freebsd || openbsd || netbsd || dragonfly || darwin

package blockuntilclosed

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func init() {
	defaultBackendFunc = func() Backend {
		return NewKQueue()
	}
}

type KQueue struct{}

func NewKQueue() *KQueue {
	return &KQueue{}
}

func (kq *KQueue) Block(ctx context.Context, fd uintptr) error {
	return darwin_kqueue(ctx, fd)
}

func (kq *KQueue) IsClosed(fd uintptr) (bool, error) {
	n, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
	// This doesn't work for TCP connections yet.
	// Moving to a unified kqueue backend may fix this.
	fmt.Println("fnctl", n, err, "errors.Is(err, unix.EBADF)", errors.Is(err, unix.EBADF))
	if errors.Is(err, unix.EBADF) { // not a valid open file descriptor.
		// I doubt this block will ever be encountered because of the guarantees provided by
		// RawConn.Control but it's here for completeness.
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

// cribbed from https://gist.github.com/juanqui/7564275
func darwin_kqueue(ctx context.Context, fd uintptr) error {
	kqfd, err := unix.Kqueue() // TODO: this kqueue should be global to avoid creating it every time
	if err != nil {
		return fmt.Errorf("unix.Kqueue(): %w", err)
	}
	defer unix.Close(kqfd)

	// pipes is a surrogate for the context so kqueue wakes up when the context is done.
	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("os.Pipe(): %w", err)
	}
	defer pipeRead.Close()
	defer pipeWrite.Close()

	stop := context.AfterFunc(ctx, func() {
		pipeWrite.Close()
	})
	defer stop()

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

	ev2 := unix.Kevent_t{
		Ident:  uint64(pipeRead.Fd()),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD | unix.EV_ENABLE | unix.EV_ONESHOT | unix.EV_DISPATCH2 | unix.EV_VANISHED,
		Fflags: unix.NOTE_NONE,
		Data:   0,
		Udata:  nil,
	}

	timeout := &unix.Timespec{
		Sec:  0,
		Nsec: 100000,
	}
	timeout = nil

	var events [1]unix.Kevent_t

	n, err := unix.Kevent(kqfd, []unix.Kevent_t{ev1, ev2}, events[:], timeout)
	if err != nil {
		return fmt.Errorf("unix.Kevent(): %w", err)
	}
	fmt.Println("got", n)
	gotEvent := events[0]

	fromSocket := uintptr(gotEvent.Ident) == fd

	// vanishedFlag := gotEvent.Flags&unix.EV_VANISHED != 0 // TODO do we need this?
	errorFlag := gotEvent.Flags&unix.EV_ERROR != 0
	eofFlag := gotEvent.Flags&unix.EV_EOF != 0

	fmt.Printf("gotEvent: %+v fromSocket=%+v\n", gotEvent, fromSocket)

	if eofFlag {
		fmt.Println("eof")
	}
	if errorFlag {
		return fmt.Errorf("ev2.Flags&unix.EV_ERROR != 0: %d", unix.Errno(gotEvent.Data))
	}

	return ctx.Err() // if we aborted the context we want to return the error
}
