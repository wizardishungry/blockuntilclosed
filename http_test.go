package blockuntilclosed

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestHTTPServer tests that a slow body will be interrupted when the client closes the connection.
// This was designed for fasthttp, golang net/http works fine with the request context.
func TestHTTPServer(t *testing.T) {
	fe := WithBackend(NewDefaultBackend())

	inHandler := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inHandler = true
		t.Log("handler called")
		b, err := io.ReadAll(r.Body)
		t.Log("read", string(b), err)
	})

	var wg sync.WaitGroup
	wg.Add(1)
	srv := httptest.NewServer(h)
	srv.Config.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		syscallConn, ok := c.(syscall.Conn)
		if !ok {
			t.Fatal("not a syscall.Conn")
			return ctx
		}

		done := fe.Done(syscallConn)

		if done == nil {
			t.Fatal("expected channel")
		}

		select {
		default:
		case <-done:
			t.Fatal("expected done to block")
		}

		go func() {
			defer wg.Done()
			<-done
			t.Log("got client hangup")
		}()

		return ctx
	}

	t.Cleanup(srv.Close)

	client := &http.Client{}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	var hijackedConn net.Conn
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		hijackedConn = conn

		return conn, err
	}

	tr := &http.Transport{}
	tr.DialContext = dialContext
	client.Transport = tr

	slowBody := &slowBody{
		callback: func() {
			hijackedConn.Close()
		},
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL, slowBody)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Logf("client.Do: %v", err)
	} else {
		defer resp.Body.Close()
	}
	if !inHandler {
		t.Fatal("handler not called")
	}

	wg.Wait()
	t.Log("done")

}

type slowBody struct {
	callback func()
}

func (s *slowBody) Read(p []byte) (n int, err error) {
	time.Sleep(1 * time.Second)
	p = []byte("hello")

	s.callback()

	return len(p), nil
}
