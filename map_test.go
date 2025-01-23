package blockuntilclosed

import (
	"net"
	"testing"
	"time"
)

func TestMapDeduped(t *testing.T) {
	be := NewDefaultBackend()
	fe := WithBackend(be)

	m := be.getMap()
	if m == nil {
		t.Skip("backend doesn't use map")
	}

	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go func() {
		sock, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(waitTime) // events take a sec to propagate
		sock.Close()
		sock.CloseRead()
		sock.CloseWrite()
		defer sock.Close()
	}()

	sock, err := l.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}

	c := fe.Done(sock)
	if c == nil {
		t.Fatal("expected channel")
	}
	cNew := fe.Done(sock)
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
	sock.CloseRead()
	sock.CloseWrite()

	time.Sleep(waitTime) // events take a sec to propagate

	select {
	case <-c:
	default:
		t.Fatal("expected close")
	}

	count = 0
	m.m.Range(func(key, value any) bool {
		count++
		return true
	})
	if count != 0 {
		t.Fatalf("expected no entries, got %d", count)
	}
}
