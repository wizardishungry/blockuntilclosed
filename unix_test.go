package blockuntilclosed

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

func TestUnix(t *testing.T) {
	ctx := context.Background()

	tmpFile, err := os.CreateTemp("", "test.sock")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if err := os.Remove(tmpFile.Name()); err != nil {
		t.Fatal(err)
	}

	l, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: tmpFile.Name(),
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
		conn, err := l.AcceptUnix()
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
		conn, err := net.DialUnix("unix", nil, l.Addr().(*net.UnixAddr))
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
