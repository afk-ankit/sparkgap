package breaker

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func waitForState[T any](t *testing.T, br *Breaker[T], state int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if br.getState() == state {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func TestExecuteSuccessWhenClosed(t *testing.T) {
	br := InitBreaker[int]("success-closed")
	br.Counter.FailureThreshold = 2

	v, err := br.Execute(func() (int, error) { return 7, nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 7 {
		t.Fatalf("unexpected value: got %d, want %d", v, 7)
	}
	if st := br.getState(); st != Closed {
		t.Fatalf("state: got %d, want Closed", st)
	}
	if f := atomic.LoadUint32(&br.Counter.Failure); f != 0 {
		t.Fatalf("failure counter: got %d, want 0", f)
	}
}

func TestFailureCountingAndOpenTransition(t *testing.T) {
	br := InitBreaker[int]("open-transition")
	br.Counter.FailureThreshold = 3
	br.Counter.RetryDuration = 50 * time.Millisecond

	// First failure: still Closed
	if _, err := br.Execute(func() (int, error) { return 0, errors.New("fail 1") }); err == nil {
		t.Fatal("expected error on first failure")
	}
	if st := br.getState(); st != Closed {
		t.Fatalf("state after 1 failure: got %d, want Closed", st)
	}
	if f := atomic.LoadUint32(&br.Counter.Failure); f != 1 {
		t.Fatalf("failure counter after 1 failure: got %d, want 1", f)
	}

	// Second failure: still Closed
	if _, err := br.Execute(func() (int, error) { return 0, errors.New("fail 2") }); err == nil {
		t.Fatal("expected error on second failure")
	}
	if st := br.getState(); st != Closed {
		t.Fatalf("state after 2 failures: got %d, want Closed", st)
	}
	if f := atomic.LoadUint32(&br.Counter.Failure); f != 2 {
		t.Fatalf("failure counter after 2 failures: got %d, want 2", f)
	}

	// Third failure: should Open and schedule retry
	if _, err := br.Execute(func() (int, error) { return 0, errors.New("fail 3") }); err == nil {
		t.Fatal("expected error on third failure")
	}
	if st := br.getState(); st != Open {
		t.Fatalf("state after reaching threshold: got %d, want Open", st)
	}

	// While Open, Execute should short-circuit
	if _, err := br.Execute(func() (int, error) { return 1, nil }); err == nil || err.Error() != "circuit breaker is open" {
		t.Fatalf("expected open error, got: %v", err)
	}

	// Wait for HalfOpen after retry duration
	if !waitForState(t, br, HalfOpen, 300*time.Millisecond) {
		t.Fatal("timeout waiting for HalfOpen state")
	}
}

func TestHalfOpenRetryFailAndSuccess(t *testing.T) {
	br := InitBreaker[int]("half-open-flow")
	br.Counter.FailureThreshold = 2
	br.Counter.RetryDuration = 40 * time.Millisecond

	// Drive to Open
	for i := 0; i < int(br.Counter.FailureThreshold); i++ {
		_, _ = br.Execute(func() (int, error) { return 0, errors.New("trigger open") })
	}
	if st := br.getState(); st != Open {
		t.Fatalf("expected Open state, got %d", st)
	}

	// Wait to HalfOpen
	if !waitForState(t, br, HalfOpen, 250*time.Millisecond) {
		t.Fatal("timeout waiting for HalfOpen state")
	}

	// In HalfOpen: fail probe -> back to Open and schedule retry again
	if _, err := br.Execute(func() (int, error) { return 0, errors.New("probe fail") }); err == nil {
		t.Fatal("expected error on half-open probe failure")
	}
	if st := br.getState(); st != Open {
		t.Fatalf("after probe failure, expected Open, got %d", st)
	}

	// Wait to HalfOpen again
	if !waitForState(t, br, HalfOpen, 250*time.Millisecond) {
		t.Fatal("timeout waiting for HalfOpen state again")
	}

	// In HalfOpen: success -> Closed and reset failures
	val, err := br.Execute(func() (int, error) { return 123, nil })
	if err != nil {
		t.Fatalf("unexpected error on half-open success: %v", err)
	}
	if val != 123 {
		t.Fatalf("unexpected value on success: got %d, want 123", val)
	}
	if st := br.getState(); st != Closed {
		t.Fatalf("after successful probe, expected Closed, got %d", st)
	}
}

func TestHalfOpenSuccessThresholdMultiple(t *testing.T) {
	br := InitBreaker[int]("half-open-success-threshold")
	br.Counter.FailureThreshold = 1
	br.Counter.RetryDuration = 30 * time.Millisecond
	br.Counter.SuccessThreshold = 2

	// Drive to Open with a single failure
	_, _ = br.Execute(func() (int, error) { return 0, errors.New("boom") })
	if st := br.getState(); st != Open {
		t.Fatalf("expected Open state, got %d", st)
	}

	// Wait to HalfOpen
	if !waitForState(t, br, HalfOpen, 200*time.Millisecond) {
		t.Fatal("timeout waiting for HalfOpen state")
	}

	// First success in HalfOpen: should remain HalfOpen (need 2 successes)
	if _, err := br.Execute(func() (int, error) { return 1, nil }); err != nil {
		t.Fatalf("unexpected error on first half-open success: %v", err)
	}
	if st := br.getState(); st != HalfOpen {
		t.Fatalf("after first success, expected HalfOpen, got %d", st)
	}

	// Second success: should transition to Closed and reset success counter
	if _, err := br.Execute(func() (int, error) { return 2, nil }); err != nil {
		t.Fatalf("unexpected error on second half-open success: %v", err)
	}
	if st := br.getState(); st != Closed {
		t.Fatalf("after second success, expected Closed, got %d", st)
	}
	if s := atomic.LoadUint32(&br.Counter.Success); s != 0 {
		t.Fatalf("success counter should reset after closing, got %d", s)
	}
}

func TestHalfOpenSuccessCarryOverAcrossRetries(t *testing.T) {
	br := InitBreaker[int]("half-open-success-carry")
	br.Counter.FailureThreshold = 1
	br.Counter.RetryDuration = 30 * time.Millisecond
	br.Counter.SuccessThreshold = 2

	// Open the circuit
	_, _ = br.Execute(func() (int, error) { return 0, errors.New("boom") })
	if st := br.getState(); st != Open {
		t.Fatalf("expected Open, got %d", st)
	}

	// Wait to HalfOpen
	if !waitForState(t, br, HalfOpen, 200*time.Millisecond) {
		t.Fatal("timeout waiting for HalfOpen state")
	}

	// One success; threshold is 2 so should stay HalfOpen and increment Success to 1
	if _, err := br.Execute(func() (int, error) { return 1, nil }); err != nil {
		t.Fatalf("unexpected error on half-open success: %v", err)
	}
	if st := br.getState(); st != HalfOpen {
		t.Fatalf("after first success, expected HalfOpen, got %d", st)
	}

	// Now a failure in HalfOpen: should go back to Open (note: Success counter is NOT reset by implementation)
	if _, err := br.Execute(func() (int, error) { return 0, errors.New("probe fail") }); err == nil {
		t.Fatal("expected error on half-open probe failure")
	}
	if st := br.getState(); st != Open {
		t.Fatalf("after probe failure, expected Open, got %d", st)
	}

	// Wait to HalfOpen again
	if !waitForState(t, br, HalfOpen, 200*time.Millisecond) {
		t.Fatal("timeout waiting for HalfOpen again")
	}

	// With Success previously incremented to 1 and not reset, a single success now should close
	if _, err := br.Execute(func() (int, error) { return 2, nil }); err != nil {
		t.Fatalf("unexpected error on second half-open window success: %v", err)
	}
	if st := br.getState(); st != Closed {
		t.Fatalf("expected Closed after second window success, got %d", st)
	}
	if s := atomic.LoadUint32(&br.Counter.Success); s != 0 {
		t.Fatalf("success counter should reset after closing, got %d", s)
	}
}

func TestHalfOpenImmediateCloseWithZeroSuccessThreshold(t *testing.T) {
	br := InitBreaker[int]("half-open-zero-threshold")
	br.Counter.FailureThreshold = 1
	br.Counter.RetryDuration = 20 * time.Millisecond
	// SuccessThreshold default is 0 from InitBreaker

	// Open the circuit
	_, _ = br.Execute(func() (int, error) { return 0, errors.New("boom") })

	// HalfOpen
	if !waitForState(t, br, HalfOpen, 200*time.Millisecond) {
		t.Fatal("timeout waiting for HalfOpen state")
	}

	// Any success should immediately close when threshold is 0
	if _, err := br.Execute(func() (int, error) { return 1, nil }); err != nil {
		t.Fatalf("unexpected error on half-open success with zero threshold: %v", err)
	}
	if st := br.getState(); st != Closed {
		t.Fatalf("expected Closed immediately when SuccessThreshold==0, got %d", st)
	}
}

func TestOpenStateImmediateError(t *testing.T) {
	br := InitBreaker[int]("open-immediate")
	br.setState(Open)

	val, err := br.Execute(func() (int, error) { return 42, nil })
	if err == nil || err.Error() != "circuit breaker is open" {
		t.Fatalf("expected open error, got: %v", err)
	}
	if val != 0 {
		t.Fatalf("expected zero value when open, got %d", val)
	}
}
