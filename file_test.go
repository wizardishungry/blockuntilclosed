package blockuntilclosed

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestFile(t *testing.T) {
	t.Skip("skipping test that doesn't work on darwin")

	const waitTime = 100 * time.Millisecond

	f, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("created", f.Name())
	name := f.Name()
	defer os.Remove(name)

	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		time.Sleep(waitTime)
		f.Close()
		t.Log("closed")
	}()

	go func() {
		defer wg.Done()
		done, err := Done(f)
		if err != nil {
			t.Fatal(err)
		}
		<-done
		t.Log("got eof")
	}()

	wg.Wait()

	dur := time.Since(start)
	if dur < waitTime {
		t.Fatalf("expected to wait at least %v, but waited %v", waitTime, dur)
	}
	t.Log("waited", dur)
}
