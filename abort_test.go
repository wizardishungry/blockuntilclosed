package blockuntilclosed

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// TestAbort checks that aborting a context will not cause <-Done(conn) to succeed.
func TestAbort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		conn, err := l.AcceptTCP()
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		select {
		case <-Done(conn):
			t.Fatal("expected to not receive done")
		case <-ctx.Done():
			t.Log("aborted")
		}

	}()

	go func() {
		defer wg.Done()
		conn, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(waitTime)
		_ = conn

		cancel()
	}()

	wg.Wait()

	dur := time.Since(start)
	if dur < waitTime {
		t.Fatalf("expected to wait at least %v, but waited %v", waitTime, dur)
	}
	t.Log("waited", dur)
}

// TestAlreadyClosed checks that passing a closed connection will:
// - Cause <-Done(conn) to succeed.
// - Not leak a map entry with a file descriptor.
func TestAlreadyClosed(t *testing.T) {
	be := NewDefaultBackend()
	fe := WithBackend(be)
	beWithMap, ok := be.(getMap)

	if !ok {
		t.Skip("backend doesn't use map")
	}
	m := beWithMap.getMap()

	l, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	conn, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	c := fe.Done(conn)

	select {
	case <-c:
		t.Log("done")
	case <-time.After(waitTime):
		t.Fatal("expected to receive done")
	}

	m.m.Range(func(key, value interface{}) bool {
		t.Fatal("expected no map entries")
		return false
	})
}
