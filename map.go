package blockuntilclosed

import (
	"log"
	"sync"

	"golang.org/x/sys/unix"
)

type closeMap struct {
	m sync.Map // map[int] *closeMapPayload
}

type closeMapPayload struct {
	c chan struct{}
}

func (cm *closeMap) Close(key int) bool {
	v, loaded := cm.m.LoadAndDelete(key)
	if !loaded {
		return false
	}
	if payload, ok := v.(*closeMapPayload); ok {
		close(payload.c)

		err := unix.Close(key) // Close the dup'd file descriptor
		if err != nil {
			log.Printf("unix.Close(%d): %v", key, err) // TODO: inject logger
		}

		return true
	}
	return false // May have already been closed. But how?
}

func (cm *closeMap) Add(key int) (loaded bool, _ *closeMapPayload) {
	c := make(chan struct{})
	payload := &closeMapPayload{
		c: c,
	}
	v, loaded := cm.m.LoadOrStore(key, payload)
	if !loaded {
		return false, payload
	}
	if p, ok := v.(*closeMapPayload); ok {
		return true, p
	}

	return false, nil // This is an error
}

func (cm *closeMap) Drain() (count int) {
	cm.m.Range(func(key, value interface{}) bool {
		closed := cm.Close(key.(int))
		if closed {
			count++
		}
		return true // continue
	})
	return count
}
