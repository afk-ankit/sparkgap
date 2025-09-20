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
	Failure         uint32
	FailureThrehold uint32
	RetryDuration   time.Duration
}

type Breaker[T any] struct {
	Name    string
	Counter Counter
	State   int
}

func (br *Breaker[T]) startRetry() {
	go func() {
		time.Sleep(br.Counter.RetryDuration)
		br.State = HalfOpen
	}()
}

func (br *Breaker[T]) Execute(fn func() (T, error)) (T, error) {
	var zero T
	switch br.State {
	case Open:
		return zero, fmt.Errorf("circuit breaker is open")
	case HalfOpen:
		res, err := fn()
		if err != nil {
			fmt.Println("Rety Unsuccessfull")
			br.failure()
			return res, err
		}
		atomic.StoreUint32(&br.Counter.Failure, 0)
		fmt.Println("Retry Successfull")
		br.State = Closed
		return res, nil
	case Closed:
		res, err := fn()
		if err != nil {
			br.failure()
			return res, err
		}
		return res, nil
	}

	if br.State == Open {
		return zero, fmt.Errorf("circuit breaker is open")
	}
	res, err := fn()
	if err != nil {
		br.failure()
		return res, err
	}
	return res, nil
}

func (br *Breaker[T]) failure() {
	if br.State == HalfOpen {
		br.State = Open
	}
	atomic.AddUint32(&br.Counter.Failure, 1)
	if br.Counter.Failure >= br.Counter.FailureThrehold {
		br.State = Open
		br.startRetry()
	}
}

func InitBreaker[T any](name string) *Breaker[T] {
	return &Breaker[T]{
		Name: name,
		Counter: Counter{
			FailureThrehold: 10,
			RetryDuration:   time.Second * 5,
		},
		State: Closed,
	}
}
