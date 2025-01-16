package blockuntilclosed

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestTCP(t *testing.T) {
	ctx := context.Background()

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

		if isClosed, err := IsClosed(conn); err != nil {
			t.Fatal(err)
		} else if isClosed {
			t.Fatal("expected conn to be open")
		}

		if err := Block(ctx, conn); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * waitTime)

		if isClosed, err := IsClosed(conn); err != nil {
			t.Fatal(err)
		} else if !isClosed {
			t.Fatal("expected conn to be closed")
		}

	}()

	go func() {
		defer wg.Done()
		conn, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		if isClosed, err := IsClosed(conn); err != nil {
			t.Fatal(err)
		} else if isClosed {
			t.Fatal("expected conn to be open")
		}

		time.Sleep(waitTime)
		t.Log("closing")
	}()

	wg.Wait()

	dur := time.Since(start)
	if dur < waitTime {
		t.Fatalf("expected to wait at least %v, but waited %v", waitTime, dur)
	}
	t.Log("waited", dur)
}

// TestTCP_block_after_close tests that block works after the connection is closed.
func TestTCP_block_after_close(t *testing.T) {
	ctx := context.Background()

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
		time.Sleep(waitTime)
		ctx, cancel := context.WithTimeout(ctx, waitTime)
		defer cancel()

		if err := Block(ctx, conn); err != nil {
			t.Fatal("expected no error, got", err)
		}
	}()

	go func() {
		defer wg.Done()
		conn, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		t.Log("closing")
		conn.Close()

		ctx, cancel := context.WithTimeout(ctx, waitTime)
		defer cancel()

		if err := Block(ctx, conn); err != nil {
			t.Fatalf("expected no error, got %v %T", err, err)
		}
	}()

	wg.Wait()

	dur := time.Since(start)
	if dur < waitTime {
		t.Fatalf("expected to wait at least %v, but waited %v", waitTime, dur)
	}
	t.Log("waited", dur)
}
