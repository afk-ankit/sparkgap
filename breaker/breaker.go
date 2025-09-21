/*
Package breaker implements a circuit breaker state machine with support for Closed, Open, and Half-Open states.
It allows wrapping function calls to prevent cascading failures and supports failure thresholds, timeouts, and recovery probes.
*/
package breaker

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
)

const (
	Closed = iota
	Open
	HalfOpen
)

type Counter struct {
	Failure                    uint32
	FailureThreshold           uint32
	RetryDuration              time.Duration
	HalfStateTotalRetryCount   uint32
	HalfStateFailureCount      uint32
	HalfStateSuccessCount      uint32
	HalfStateFailurePercentage uint32
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

func stateToString(state int) string {
	switch state {
	case Closed:
		return "Closed"
	case Open:
		return "Open"
	case HalfOpen:
		return "Half-Open"
	default:
		return fmt.Sprintf("Unknown(%d)", state)
	}
}

func (br *Breaker[T]) LogState() {
	st := br.getState()
	failure := atomic.LoadUint32(&br.Counter.Failure)
	hFail := atomic.LoadUint32(&br.Counter.HalfStateFailureCount)
	hSucc := atomic.LoadUint32(&br.Counter.HalfStateSuccessCount)

	tw := table.NewWriter()
	tw.SetOutputMirror(os.Stdout)
	tw.SetStyle(table.StyleColoredBlackOnCyanWhite)
	tw.AppendHeader(table.Row{"Circuit Breaker", br.Name})
	tw.AppendRow(table.Row{"State", stateToString(st)})
	if br.TimeOutDuration > 0 {
		tw.AppendRow(table.Row{"Timeout", br.TimeOutDuration})
	}
	tw.AppendRow(table.Row{"Failure (current/threshold)", fmt.Sprintf("%d / %d", failure, br.Counter.FailureThreshold)})
	tw.AppendRow(table.Row{"Retry Duration", br.Counter.RetryDuration})
	tw.AppendRow(table.Row{"Half-Open total probes", br.Counter.HalfStateTotalRetryCount})
	tw.AppendRow(table.Row{"Half-Open success count", hSucc})
	tw.AppendRow(table.Row{"Half-Open failure count", hFail})
	tw.AppendRow(table.Row{"Half-Open fail % threshold", fmt.Sprintf("%d%%", br.Counter.HalfStateFailurePercentage)})

	tw.Render()
}

func (br *Breaker[T]) Execute(fn func() (T, error)) (T, error) {
	var zero T
	switch br.getState() {
	case Open:
		return zero, fmt.Errorf("circuit breaker is open")
	case HalfOpen:
		res, err := fn()
		if err != nil {
			// increment half state failureCount
			br.setHalfStateCount(0, 1)
			return res, err
		}
		// increment half State successCount
		br.setHalfStateCount(1, 0)
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

func (br *Breaker[T]) setHalfStateCount(successCount uint32, failureCount uint32) {
	if br.getState() != HalfOpen {
		return
	}
	atomic.AddUint32(&br.Counter.HalfStateFailureCount, failureCount)
	atomic.AddUint32(&br.Counter.HalfStateSuccessCount, successCount)
	atomicFailure := atomic.LoadUint32(&br.Counter.HalfStateFailureCount)
	atomicSuccess := atomic.LoadUint32(&br.Counter.HalfStateSuccessCount)

	if atomicFailure+atomicSuccess == br.Counter.HalfStateTotalRetryCount {
		failurePercentage := uint32(float64(atomicFailure) / float64(br.Counter.HalfStateTotalRetryCount) * 100)
		if failurePercentage >= br.Counter.HalfStateFailurePercentage {
			br.setState(Open)
			br.startRetry()
		} else {
			atomic.StoreUint32(&br.Counter.Failure, 0)
			br.setState(Closed)
		}
		atomic.StoreUint32(&br.Counter.HalfStateFailureCount, 0)
		atomic.StoreUint32(&br.Counter.HalfStateSuccessCount, 0)
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
			FailureThreshold:           5,
			RetryDuration:              time.Second * 5,
			HalfStateTotalRetryCount:   10,
			HalfStateFailurePercentage: 30,
		},
		State: Closed,
	}
}
