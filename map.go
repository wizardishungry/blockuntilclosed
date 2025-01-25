package blockuntilclosed

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

type closeMap struct {
	m sync.Map // map[int] *closeMapPayload
}

type closeMapPayload struct {
	cancelCause context.CancelCauseFunc
	stop        func() bool
}

func (payload *closeMapPayload) Close(key int, reason error) {
	err := unix.Close(key) // Close the dup'd file descriptor
	if err != nil {
		err := fmt.Errorf("%w: %w", ErrUnixCloseDup, err)
		reason = errors.Join(reason, err)
	}

	if payload.stop != nil {
		// call stop before canceling the context so the AfterFunc doesn't get called.
		payload.stop()
	}

	payload.cancelCause(reason)
}

func (cm *closeMap) Close(key int, reason error) bool {
	v, loaded := cm.m.LoadAndDelete(key)
	if !loaded {
		return false
	}
	if payload, ok := v.(*closeMapPayload); ok {
		payload.Close(key, reason)
		return true
	}
	return false // May have already been closed. But how?
}

func (cm *closeMap) Add(key int, ctx context.Context, cancelCause context.CancelCauseFunc) (loaded bool, _ *closeMapPayload) {
	payload := &closeMapPayload{
		cancelCause: cancelCause,
	}
	v, loaded := cm.m.LoadOrStore(key, payload)
	if !loaded {
		// This is a goroutine. It is not ideal.
		// The happy path deschedules it.
		stop := context.AfterFunc(ctx, func() {
			cm.Close(key, context.Cause(ctx))
		})
		payload.stop = stop

		return false, payload
	}
	if p, ok := v.(*closeMapPayload); ok {
		return true, p
	}

	return false, nil // This is an error
}

func (cm *closeMap) Drain() (count int) {
	cm.m.Range(func(key, value interface{}) bool {
		closed := cm.Close(key.(int), ErrBackendClosed)
		if closed {
			count++
		}
		return true // continue
	})
	return count
}
