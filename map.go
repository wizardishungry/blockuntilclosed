package blockuntilclosed

import "sync"

type closeMap[T comparable] struct {
	m sync.Map // m map[T]chan struct{}
}

func newCloseMap[T comparable]() *closeMap[T] {
	return &closeMap[T]{
		m: sync.Map{},
	}
}

func (cm *closeMap[T]) Close(key T) bool {
	v, loaded := cm.m.LoadAndDelete(key)
	if !loaded {
		return false
	}
	if c, ok := v.(chan struct{}); ok {
		close(c)
		return true
	}
	return false // May have already been closed
}

func (cm *closeMap[T]) Add(key T) (loaded bool, c chan struct{}) {
	c = make(chan struct{})
	v, loaded := cm.m.LoadOrStore(key, c)
	if !loaded {
		return false, c
	}
	if c, ok := v.(chan struct{}); ok {
		return true, c
	}
	return false, nil // This is an error
}
