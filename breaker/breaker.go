// Package breaker implements a circuit breaker state machine with support
// for Closed, Open, and Half-Open states. It allows wrapping function calls
// to prevent cascading failures and supports failure thresholds, timeouts,
// and recovery probes.
package breaker

import (
	"fmt"
	"sync/atomic"
	"time"
)

const (
	Closed = iota
	Open
	HalfOpen
)

type Counter struct {
	Success         uint32
	Failure         uint32
	FailureThrehold uint32
	RetryThreshold  time.Time
}

type Breaker[T any] struct {
	Name    string
	Counter Counter
	State   int
}

func (br *Breaker[T]) Execute(fn func() (T, error)) (T, error) {
	var zero T
	if br.State == Open {
		return zero, fmt.Errorf("circuit braker is open")
	}
	res, err := fn()
	if err != nil {
		br.failure()
		return res, err
	}
	br.success()
	return res, nil
}

func (br *Breaker[T]) success() {
	atomic.AddUint32(&br.Counter.Success, 1)
}

func (br *Breaker[T]) failure() {
	atomic.AddUint32(&br.Counter.Failure, 1)
	if br.Counter.Failure >= br.Counter.FailureThrehold {
		br.State = Open
	}
}
