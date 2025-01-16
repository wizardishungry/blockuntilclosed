package blockuntilclosed

import (
	"net"
	"testing"
	"time"
)

func TestMapDeduped(t *testing.T) {
	oldBackend := DefaultBackend()
	t.Cleanup(func() {
		defaultBackend = oldBackend
	})

	defaultBackend = NewDefaultBackend()

	m := defaultBackend.getMap()
	if m == nil {
		t.Skip("backend doesn't use map")
	}

	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go func() {
		net.Dial("tcp", l.Addr().String())
	}()

	sock, err := l.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}

	c := Done(sock)
	if c == nil {
		t.Fatal("expected channel")
	}
	cNew := Done(sock)
	if c != cNew {
		t.Fatal("expected same channel")
	}

	var count int
	m.m.Range(func(key, value any) bool {
		count++
		return true
	})
	if count != 1 {
		t.Fatalf("expected one entry, got %d", count)
	}

	select {
	case <-c:
		t.Fatal("unexpected close")
	default:
	}
	sock.Close()

	time.Sleep(waitTime) // events take a sec to propagate

	select {
	case <-c:
	default:
		t.Fatal("expected close")
	}
}
