// Package breaker implements a circuit breaker state machine with support
// for Closed, Open, and Half-Open states. It allows wrapping function calls
// to prevent cascading failures and supports failure thresholds, timeouts,
// and recovery probes.
package breaker

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	Closed = iota
	Open
	HalfOpen
)

type Counter struct {
	Failure          uint32
	FailureThreshold uint32
	RetryDuration    time.Duration
	Success          uint32
	SuccessThreshold uint32
}

type Breaker[T any] struct {
	Name            string
	Counter         Counter
	State           int
	TimeOutDuration time.Duration
	mu              sync.RWMutex
}

func (br *Breaker[T]) startRetry() {
	go func() {
		time.Sleep(br.Counter.RetryDuration)
		br.setState(HalfOpen)
	}()
}

func (br *Breaker[T]) setState(state int) {
	br.mu.Lock()
	br.State = state
	br.mu.Unlock()
}

func (br *Breaker[T]) getState() int {
	br.mu.RLock()
	defer br.mu.RUnlock()
	return br.State
}

func (br *Breaker[T]) Execute(fn func() (T, error)) (T, error) {
	var zero T
	switch br.getState() {
	case Open:
		return zero, fmt.Errorf("circuit breaker is open")
	case HalfOpen:
		res, err := fn()
		if err != nil {
			br.failure()
			return res, err
		}
		br.success()
		return res, err
	case Closed:
		res, err := fn()
		if err != nil {
			br.failure()
			return res, err
		}
		return res, nil
	}
	return zero, nil
}

func (br *Breaker[T]) success() {
	if br.getState() != HalfOpen {
		return
	}
	atomic.AddUint32(&br.Counter.Success, 1)
	if atomic.LoadUint32(&br.Counter.Success) >= br.Counter.SuccessThreshold {
		br.setState(Closed)
		atomic.StoreUint32(&br.Counter.Success, 0)
	}
}

func (br *Breaker[T]) failure() {
	if br.getState() == HalfOpen {
		br.setState(Open)
	}
	atomic.AddUint32(&br.Counter.Failure, 1)
	if atomic.LoadUint32(&br.Counter.Failure) >= br.Counter.FailureThreshold {
		br.State = Open
		br.startRetry()
	}
}

func InitBreaker[T any](name string) *Breaker[T] {
	return &Breaker[T]{
		Name: name,
		Counter: Counter{
			FailureThreshold: 5,
			SuccessThreshold: 0,
			RetryDuration:    time.Second * 5,
		},
		State: Closed,
	}
}
