package blockuntilclosed

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// TestAbort checks that aborting a context will stop Block from waiting.
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

	const waitTime = 100 * time.Millisecond
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

		if isClosed, err := IsClosed(conn); err != nil {
			t.Fatal(err)
		} else if isClosed {
			t.Fatal("expected conn to be open")
		}

		if err := Block(ctx, conn); err == nil {
			t.Fatal("expected an error from context cancel")
		}

		if isClosed, err := IsClosed(conn); err != nil {
			t.Fatal(err)
		} else if isClosed {
			t.Fatal("expected conn to be open")
		}

	}()

	go func() {
		defer wg.Done()
		conn, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(waitTime)

		if isClosed, err := IsClosed(conn); err != nil {
			t.Fatal(err)
		} else if isClosed {
			t.Fatal("expected conn to be open")
		}

		cancel()
	}()

	wg.Wait()

	dur := time.Since(start)
	if dur < waitTime {
		t.Fatalf("expected to wait at least %v, but waited %v", waitTime, dur)
	}
	t.Log("waited", dur)
}
