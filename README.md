# Sparkgap ⚡

A production-ready Circuit Breaker for Go with built-in jittered backoff strategies. Designed to make your services resilient, fault-tolerant, and herd-resistant when facing downstream failures.

## About the Project

**Sparkgap** is designed to protect your services from failures in downstream dependencies. By implementing jittered backoff strategies and robust circuit breaking mechanisms, it enhances the resilience and fault-tolerance of your applications. Whether you are building microservices or large-scale distributed systems, sparkgap helps ensure stability and reliability in the face of unpredictable failures.

> ⚡ This is a very initial stage project. Keep watching, things are about to get electrifying!

## Installation

Clone the repo and install dependencies:

```sh
git clone https://github.com/afk-ankit/sparkgap.git
cd sparkgap
go mod tidy
```

## Setting up Pre-commit Hooks

Pre-commit hooks help keep your codebase clean and your teammates happy. Here’s how to set it up on Linux and macOS:

1. Install pre-commit (if you don’t have it):
   ```sh
   pip install pre-commit
   ```
2. Install the hook:
   ```sh
   pre-commit install
   ```

### Windows Users

If you’re on Windows... just relax, enjoy the show, and maybe don’t contribute. (We still love you, but your shell scripts scare us.)

---

Stay tuned for more features, docs, and memes. Contributions welcome (unless you’re on Windows 😉).
