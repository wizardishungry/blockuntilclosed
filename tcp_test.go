package blockuntilclosed

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"sync"
	"testing"
	"time"
)

const waitTime = 100 * time.Millisecond

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

		subCtx := WithContext(ctx, conn)

		select {
		case <-Done(conn):
			t.Log("got eof")
		case <-ctx.Done():
			t.Fatal("expected context to not be done")
		}

		select {
		case <-subCtx.Done():
			t.Log("got subCtx done")
			err := subCtx.Err()
			if err == nil {
				t.Fatal("expected error")
			} else {
				t.Log("got error", err)
			}
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

func BenchmarkTCP(b *testing.B) {
	oldBackend := DefaultBackend()
	b.Cleanup(func() {
		defaultBackend = oldBackend
	})

	test := func(b *testing.B, doDone, waitDone bool) {
		be := NewDefaultBackend()
		be.SetLogger(log.New(io.Discard, "", 0))
		defaultBackend = be

		// Start a TCP server
		ln, err := net.ListenTCP("tcp", &net.TCPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: 0,
		})
		if err != nil {
			b.Fatalf("failed to start TCP server: %v", err)
		}
		defer ln.Close()

		// Accept connections in a separate goroutine
		go func() {
			for {
				conn, err := ln.AcceptTCP()
				if err != nil {
					return
				}
				go handleConnection(conn, doDone, waitDone)
			}
		}()

		// Run the benchmark
		b.ResetTimer()

		for b.Loop() {
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				b.Fatalf("failed to connect to TCP server: %v", err)
			}

			_, err = conn.Write([]byte("hello"))
			if err != nil {
				b.Fatalf("failed to write to TCP server: %v", err)
			}

			buf := make([]byte, 5)
			_, err = conn.Read(buf)
			if err != nil {
				b.Fatalf("failed to read from TCP server: %v", err)
			}

			conn.Close()
		}
	}

	b.Run("baseline", func(b *testing.B) {
		test(b, false, false)
	})

	b.Run("doDone", func(b *testing.B) {
		test(b, true, false)
	})
	b.Run("waitDone", func(b *testing.B) {
		test(b, false, true)
	})
}

func handleConnection(conn *net.TCPConn, doDone, waitDone bool) {
	defer conn.Close()

	if doDone {
		done := Done(conn)
		defer func() {
			select {
			case <-done:
			default:
			}
		}()
	} else if waitDone {
		done := Done(conn)
		defer func() {
			<-done
		}()
	}

	buf := make([]byte, 5)
	conn.Read(buf)
	conn.Write(buf)
}

// TestTCP_EOF checks that Done() isn't called until actual EOF (no more data in receive buffer).
func TestTCP_EOF(t *testing.T) {
	ctx := context.Background()

	l, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	const GigaByte = 1 << 30
	sendBuf := make([]byte, GigaByte)
	// fill sendBuf with data
	for i := 0; i < len(sendBuf); i++ {
		sendBuf[i] = byte(i % 256)
	}

	var recvBuf []byte

	go func() {
		defer wg.Done()
		conn, err := l.AcceptTCP()
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		subCtx := WithContext(ctx, conn)

		for subCtx.Err() == nil {
			buf := make([]byte, 4096)
			n, err := conn.Read(buf)
			if err != nil {
				t.Log("read error", err, n)
				break
			}
			recvBuf = append(recvBuf, buf[:n]...)
		}

		t.Log("subCtx error", context.Cause(subCtx))
	}()

	go func() {
		defer wg.Done()
		conn, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		n, err := io.Copy(conn, bytes.NewReader(sendBuf))
		if err != nil {
			t.Fatal(err)
		}
		if n != int64(len(sendBuf)) {
			t.Fatalf("expected to send %d bytes, but sent %d", len(sendBuf), n)
		}

		t.Log("closing")
	}()

	wg.Wait()

	equal := bytes.Equal(sendBuf, recvBuf)
	if !equal {
		t.Fatal("received data is not equal to sent data")
	}
	time.Sleep(waitTime)
}
