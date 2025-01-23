//go:build freebsd || openbsd || netbsd || dragonfly || darwin

package blockuntilclosed

import (
	"net"
	"sync"
	"testing"
	"time"
)

func TestKqueueClose(t *testing.T) {
	kq := NewKQueue()
	err := kq.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestKqueueDoStuffAndClose(t *testing.T) {
	kq := NewKQueue()
	fe := WithBackend(kq)

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

	go func() {
		defer wg.Done()
		conn, err := l.AcceptTCP()
		if err != nil {
			t.Fatal(err)
		}

		<-fe.Done(conn)
		t.Log("got eof")

	}()

	go func() {
		defer wg.Done()
		conn, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		time.Sleep(waitTime)
		t.Log("closing kqueue")

		err = kq.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()

	wg.Wait()

}
