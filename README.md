# Sparkgap ⚡

A production-ready Circuit Breaker for Go with built-in jittered backoff strategies. Designed to make your services resilient, fault-tolerant, and herd-resistant when facing downstream failures.

## About the Project

**Sparkgap** is designed to protect your services from failures in downstream dependencies. By implementing jittered backoff strategies and robust circuit breaking mechanisms, it enhances the resilience and fault-tolerance of your applications. Whether you are building microservices or large-scale distributed systems, sparkgap helps ensure stability and reliability in the face of unpredictable failures.

> ⚡ This is an early-stage project. The API may evolve feedback and contributions are very welcome.

## Install

Use Sparkgap in your Go project as a library:

```sh
go get github.com/afk-ankit/sparkgap@latest
```

Then import the package:

```go
import "github.com/afk-ankit/sparkgap"
```

For local development of this repository, see the "Local development" section below.

## Quick start

Here’s a minimal example showing how to wrap a flaky dependency with a circuit breaker. The breaker supports Closed → Open → Half-Open state transitions with configurable failure thresholds and retry durations.

```go
package main

import (
   "fmt"
   "time"

   "github.com/afk-ankit/sparkgap"
)

// Simulated downstream call
func accounts(name string, broke bool) (string, error) {
   if broke {
      return "", fmt.Errorf("service broke")
   }
   return fmt.Sprintf("Hi %s", name), nil
}

func main() {
   // Create a breaker for a string-returning dependency.
   // Pass nil to use defaults or provide a *sparkgap.BreakerConfig to customize.
   br := sparkgap.InitBreaker[string]("accounts", &sparkgap.BreakerConfig{
      FailureThreshold: 3,            // after 3 consecutive failures → Open
      RetryInterval:    2 * time.Second, // how long to wait before a Half-Open probe
      // HalfOpenMaxProbes:         10,
      // HalfOpenMaxFailurePercent: 30,
      // Timeout:                   0,
   })

   broke := false

   // Flip the dependency state to simulate failures and recovery
   go func() {
      time.Sleep(3 * time.Second)
      broke = true
      time.Sleep(6 * time.Second)
      broke = false
   }()

   for i := 0; i < 10; i++ {
      val, err := br.Execute(func() (string, error) {
         return accounts("ankit", broke)
      })
      if err != nil {
         // When Open, the breaker returns an error immediately
         fmt.Println("error:", err)
         time.Sleep(500 * time.Millisecond)
         continue
      }
      fmt.Println("response:", val)
      time.Sleep(500 * time.Millisecond)
   }
}
```

### Configuration notes

- FailureThreshold: number of consecutive failures in Closed state before transitioning to Open.
- RetryInterval: how long the breaker stays Open before moving to Half-Open to probe recovery.
- In Half-Open, a success closes the circuit and resets the failure counter; a failure re-opens it and schedules another retry window.

## Examples

There’s a runnable example at `examples/main.go`.

To run it locally:

```sh
go run ./examples
```

## Local development

Clone the repo and install module deps:

```sh
git clone https://github.com/afk-ankit/sparkgap.git
cd sparkgap
go mod tidy
```

See [contribution.md](contribution.md) for testing, pre-commit hooks, and PR guidelines.

---

Stay tuned for more features, docs, and memes. Contributions welcome!
