/*
Package sparkgap implements a circuit breaker state machine with support for Closed, Open, and Half-Open states.
It allows wrapping function calls to prevent cascading failures and supports failure thresholds, timeouts, and recovery probes.
*/
package sparkgap

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
)

const (
	stateClosed = iota
	stateOpen
	stateHalfOpen
)

const (
	defaultFailureThreshold          uint32 = 5
	defaultHalfOpenProbes            uint32 = 10
	defaultHalfOpenMaxFailurePercent uint32 = 30
	defaultRetryInterval                    = 5 * time.Second
)

type BreakerConfig struct {
	FailureThreshold          uint32
	RetryInterval             time.Duration
	HalfOpenMaxProbes         uint32
	HalfOpenMaxFailurePercent uint32
	Timeout                   time.Duration
}

func applyDefaults(c *BreakerConfig) {
	if c.FailureThreshold == 0 {
		c.FailureThreshold = defaultFailureThreshold
	}
	if c.RetryInterval <= 0 {
		c.RetryInterval = defaultRetryInterval
	}
	if c.HalfOpenMaxProbes == 0 {
		c.HalfOpenMaxProbes = defaultHalfOpenProbes
	}
	if c.HalfOpenMaxFailurePercent == 0 || c.HalfOpenMaxFailurePercent > 100 {
		c.HalfOpenMaxFailurePercent = defaultHalfOpenMaxFailurePercent
	}
}

type counter struct {
	failureCount              uint32
	failureThreshold          uint32
	retryInterval             time.Duration
	halfOpenMaxProbes         uint32
	halfOpenFailureCount      uint32
	halfOpenSuccessCount      uint32
	halfOpenMaxFailurePercent uint32
}

type breaker[T any] struct {
	name    string
	counter counter
	state   int
	timeout time.Duration
	mu      sync.RWMutex
}

func (br *breaker[T]) startRetry() {
	go func() {
		time.Sleep(br.counter.retryInterval)
		br.setState(stateHalfOpen)
	}()
}

func (br *breaker[T]) setState(state int) {
	br.mu.Lock()
	br.state = state
	br.mu.Unlock()
}

func (br *breaker[T]) getState() int {
	br.mu.RLock()
	defer br.mu.RUnlock()
	return br.state
}

func stateToString(state int) string {
	switch state {
	case stateClosed:
		return "Closed"
	case stateOpen:
		return "Open"
	case stateHalfOpen:
		return "Half-Open"
	default:
		return fmt.Sprintf("Unknown(%d)", state)
	}
}

func (br *breaker[T]) LogState() {
	st := br.getState()
	failures := atomic.LoadUint32(&br.counter.failureCount)
	hFail := atomic.LoadUint32(&br.counter.halfOpenFailureCount)
	hSucc := atomic.LoadUint32(&br.counter.halfOpenSuccessCount)

	tw := table.NewWriter()
	tw.SetOutputMirror(os.Stdout)
	tw.SetStyle(table.StyleColoredBlackOnCyanWhite)
	tw.AppendHeader(table.Row{"Circuit Breaker", br.name})
	tw.AppendRow(table.Row{"State", stateToString(st)})
	if br.timeout > 0 {
		tw.AppendRow(table.Row{"Timeout", br.timeout})
	}
	tw.AppendRow(table.Row{"Failure (current/threshold)", fmt.Sprintf("%d / %d", failures, br.counter.failureThreshold)})
	tw.AppendRow(table.Row{"Retry Interval", br.counter.retryInterval})
	tw.AppendRow(table.Row{"Half-Open max probes", br.counter.halfOpenMaxProbes})
	tw.AppendRow(table.Row{"Half-Open success count", hSucc})
	tw.AppendRow(table.Row{"Half-Open failure count", hFail})
	tw.AppendRow(table.Row{"Half-Open max failure %", fmt.Sprintf("%d%%", br.counter.halfOpenMaxFailurePercent)})

	tw.Render()
}

/*
Execute wraps the provided function call with circuit breaker logic.
It returns an error if the breaker is open, tracks failures and successes in half-open state,
and resets failure count on successful calls in closed state.
*/
func (br *breaker[T]) Execute(fn func() (T, error)) (T, error) {
	var zero T
	switch br.getState() {
	case stateOpen:
		return zero, fmt.Errorf("circuit breaker is open")
	case stateHalfOpen:
		res, err := fn()
		if err != nil {
			br.recordHalfOpenResult(false)
			return res, err
		}
		br.recordHalfOpenResult(true)
		return res, err
	case stateClosed:
		res, err := fn()
		if err != nil {
			br.failure()
			return res, err
		}
		return res, nil
	}
	return zero, nil
}

func (br *breaker[T]) recordHalfOpenResult(success bool) {
	if br.getState() != stateHalfOpen {
		return
	}
	if success {
		atomic.AddUint32(&br.counter.halfOpenSuccessCount, 1)
	} else {
		atomic.AddUint32(&br.counter.halfOpenFailureCount, 1)
	}

	fail := atomic.LoadUint32(&br.counter.halfOpenFailureCount)
	succ := atomic.LoadUint32(&br.counter.halfOpenSuccessCount)

	if fail+succ == br.counter.halfOpenMaxProbes {
		failurePercent := uint32(float64(fail) / float64(br.counter.halfOpenMaxProbes) * 100)
		if failurePercent >= br.counter.halfOpenMaxFailurePercent {
			br.setState(stateOpen)
			br.startRetry()
		} else {
			atomic.StoreUint32(&br.counter.failureCount, 0)
			br.setState(stateClosed)
		}
		atomic.StoreUint32(&br.counter.halfOpenFailureCount, 0)
		atomic.StoreUint32(&br.counter.halfOpenSuccessCount, 0)
	}
}

func (br *breaker[T]) failure() {
	atomic.AddUint32(&br.counter.failureCount, 1)
	if atomic.LoadUint32(&br.counter.failureCount) >= br.counter.failureThreshold {
		br.setState(stateOpen)
		br.startRetry()
	}
}

/*
InitBreaker initializes a new circuit breaker with configurable values via options.
Defaults are applied if not provided.
Backward compatible: callers can pass only the name without options.
*/
func InitBreaker[T any](name string, cfg *BreakerConfig) *breaker[T] {
	if name == "" {
		name = "breaker"
	}
	applyDefaults(cfg)

	return &breaker[T]{
		name: name,
		counter: counter{
			failureThreshold:          cfg.FailureThreshold,
			retryInterval:             cfg.RetryInterval,
			halfOpenMaxProbes:         cfg.HalfOpenMaxProbes,
			halfOpenMaxFailurePercent: cfg.HalfOpenMaxFailurePercent,
		},
		timeout: cfg.Timeout,
		state:   stateClosed,
	}
}
