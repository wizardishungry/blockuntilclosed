package blockuntilclosed

import (
	"runtime"
	"sync"
	"syscall"
)

type closeMap struct {
	m sync.Map // map[uintptr] *closeMapPayload
}

type closeMapPayload struct {
	sconn syscall.RawConn // keep a reference to the syscall.RawConn so the finalizer doesn't run before the callback; fingers crossed.
	c     chan struct{}
}

func (cm *closeMap) Close(key uintptr) bool {
	v, loaded := cm.m.LoadAndDelete(key)
	if !loaded {
		return false
	}
	if payload, ok := v.(*closeMapPayload); ok {
		close(payload.c)
		return true
	}
	return false // May have already been closed
}

func (cm *closeMap) Add(key uintptr, sconn syscall.RawConn) (loaded bool, _ *closeMapPayload) {
	c := make(chan struct{})
	payload := &closeMapPayload{
		sconn: sconn,
		c:     c,
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
