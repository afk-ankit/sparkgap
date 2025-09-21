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
	closed = iota
	open
	halfOpen
)

const (
	defaultFailureThreshold          uint32 = 5
	defaultHalfOpenProbes            uint32 = 10
	defaultHalfOpenFailurePercentage uint32 = 30
	defaultRetryFrequency                   = 5 * time.Second
)

type BreakerConfig struct {
	FailureThreshold           uint32
	RetryFrequency             time.Duration
	HalfStateTotalRetryCount   uint32
	HalfStateFailurePercentage uint32
	Timeout                    time.Duration
}

func applyDefaults(c *BreakerConfig) {
	if c.FailureThreshold == 0 {
		c.FailureThreshold = defaultFailureThreshold
	}
	if c.RetryFrequency <= 0 {
		c.RetryFrequency = defaultRetryFrequency
	}
	if c.HalfStateTotalRetryCount == 0 {
		c.HalfStateTotalRetryCount = defaultHalfOpenProbes
	}
	if c.HalfStateFailurePercentage == 0 || c.HalfStateFailurePercentage > 100 {
		c.HalfStateFailurePercentage = defaultHalfOpenFailurePercentage
	}
}

type counter struct {
	failure                    uint32
	failureThreshold           uint32
	retryFrequency             time.Duration
	halfStateTotalRetryCount   uint32
	halfStateFailureCount      uint32
	halfStateSuccessCount      uint32
	halfStateFailurePercentage uint32
}

type breaker[T any] struct {
	name            string
	counter         counter
	state           int
	timeoutDuration time.Duration
	mu              sync.RWMutex
}

func (br *breaker[T]) startRetry() {
	go func() {
		time.Sleep(br.counter.retryFrequency)
		br.setState(halfOpen)
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
	case closed:
		return "Closed"
	case open:
		return "Open"
	case halfOpen:
		return "Half-Open"
	default:
		return fmt.Sprintf("Unknown(%d)", state)
	}
}

func (br *breaker[T]) LogState() {
	st := br.getState()
	failure := atomic.LoadUint32(&br.counter.failure)
	hFail := atomic.LoadUint32(&br.counter.halfStateFailureCount)
	hSucc := atomic.LoadUint32(&br.counter.halfStateSuccessCount)

	tw := table.NewWriter()
	tw.SetOutputMirror(os.Stdout)
	tw.SetStyle(table.StyleColoredBlackOnCyanWhite)
	tw.AppendHeader(table.Row{"Circuit Breaker", br.name})
	tw.AppendRow(table.Row{"State", stateToString(st)})
	if br.timeoutDuration > 0 {
		tw.AppendRow(table.Row{"Timeout", br.timeoutDuration})
	}
	tw.AppendRow(table.Row{"Failure (current/threshold)", fmt.Sprintf("%d / %d", failure, br.counter.failureThreshold)})
	tw.AppendRow(table.Row{"Retry Duration", br.counter.retryFrequency})
	tw.AppendRow(table.Row{"Half-Open total probes", br.counter.halfStateTotalRetryCount})
	tw.AppendRow(table.Row{"Half-Open success count", hSucc})
	tw.AppendRow(table.Row{"Half-Open failure count", hFail})
	tw.AppendRow(table.Row{"Half-Open fail % threshold", fmt.Sprintf("%d%%", br.counter.halfStateFailurePercentage)})

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
	case open:
		return zero, fmt.Errorf("circuit breaker is open")
	case halfOpen:
		res, err := fn()
		if err != nil {
			br.setHalfStateCount(0, 1)
			return res, err
		}
		br.setHalfStateCount(1, 0)
		return res, err
	case closed:
		res, err := fn()
		if err != nil {
			br.failure()
			return res, err
		}
		return res, nil
	}
	return zero, nil
}

func (br *breaker[T]) setHalfStateCount(successCount uint32, failureCount uint32) {
	if br.getState() != halfOpen {
		return
	}
	atomic.AddUint32(&br.counter.halfStateFailureCount, failureCount)
	atomic.AddUint32(&br.counter.halfStateSuccessCount, successCount)
	atomicFailure := atomic.LoadUint32(&br.counter.halfStateFailureCount)
	atomicSuccess := atomic.LoadUint32(&br.counter.halfStateSuccessCount)

	if atomicFailure+atomicSuccess == br.counter.halfStateTotalRetryCount {
		failurePercentage := uint32(float64(atomicFailure) / float64(br.counter.halfStateTotalRetryCount) * 100)
		if failurePercentage >= br.counter.halfStateFailurePercentage {
			br.setState(open)
			br.startRetry()
		} else {
			atomic.StoreUint32(&br.counter.failure, 0)
			br.setState(closed)
		}
		atomic.StoreUint32(&br.counter.halfStateFailureCount, 0)
		atomic.StoreUint32(&br.counter.halfStateSuccessCount, 0)
	}
}

func (br *breaker[T]) failure() {
	atomic.AddUint32(&br.counter.failure, 1)
	if atomic.LoadUint32(&br.counter.failure) >= br.counter.failureThreshold {
		br.setState(open)
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
			failureThreshold:           cfg.FailureThreshold,
			retryFrequency:             cfg.RetryFrequency,
			halfStateTotalRetryCount:   cfg.HalfStateTotalRetryCount,
			halfStateFailurePercentage: cfg.HalfStateFailurePercentage,
		},
		timeoutDuration: cfg.Timeout,
		state:           closed,
	}
}
