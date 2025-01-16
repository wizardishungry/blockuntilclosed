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

		select {
		case <-Done(conn):
			t.Log("got eof")
		case <-ctx.Done():
			t.Fatal("expected context to not be done")
		}

	}()

	go func() {
		defer wg.Done()
		conn, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

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
