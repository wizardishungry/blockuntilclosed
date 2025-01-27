package blockuntilclosed

import (
	"net"
	"testing"
	"time"
)

type getMap interface {
	getMap() *closeMap
}

func TestMapDeduped(t *testing.T) {
	be := NewDefaultBackend()
	fe := NewFrontend(be)
	beWithMap, ok := be.(getMap)

	if !ok {
		t.Skip("backend doesn't use map")
	}
	m := beWithMap.getMap()

	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	goAhead := make(chan struct{})

	go func() {
		defer func() {
			goAhead <- struct{}{}
			t.Log("client sent go ahead")
		}()
		sock, err := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
		if err != nil {
			t.Fatal(err)
		}
		defer sock.Close()
		t.Log("client waiting for go ahead")
		<-goAhead
		t.Log("client closing dialed connection")
		sock.Close()
	}()

	sock, err := l.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	defer sock.Close()

	t.Log("accepted", sock.RemoteAddr())

	c := fe.Done(sock)
	if c == nil {
		t.Fatal("expected channel")
	}

	if cNew := fe.Done(sock); c != cNew {
		t.Log("expected same channel") // when we added dup'd fd, we no longer get the same channel.
	}

	var count int
	m.m.Range(func(key, value any) bool {
		count++
		return true
	})
	if count != 1 {
		t.Logf("expected one entry, got %d", count) // when we added dup'd fd, we no longer get the same channel.
	}

	select {
	case <-c:
		t.Fatal("unexpected close")
	default:
	}

	goAhead <- struct{}{}
	t.Log("server waiting for go ahead")
	<-goAhead
	time.Sleep(10 * time.Millisecond) // give the client a chance to close the connection

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

	t.Log("closing backend")
	if err := be.Close(); err != nil {
		t.Fatal(err)
	}
}
