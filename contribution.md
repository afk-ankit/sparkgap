# Contribution Guide

Thanks for your interest in contributing to Sparkgap! This guide will help you set up your environment, run tests and examples, and submit high-quality pull requests.

## Table of contents

- [Project setup](#project-setup)
- [Running tests](#running-tests)
- [Running the example](#running-the-example)
- [Code style and checks](#code-style-and-checks)
- [Pre-commit hooks](#pre-commit-hooks)
- [Branching and PR guidelines](#branching-and-pr-guidelines)
- [Filing issues](#filing-issues)

## Project setup

Requirements:

- Go 1.21+ (module targets may compile on newer versions; see `go.mod`)
- Git
- Optional: Python 3 with `pre-commit` if you want local git hooks

Clone the repository and install dependencies:

```sh
git clone https://github.com/afk-ankit/sparkgap.git
cd sparkgap
go mod tidy
```

## Running tests

Unit tests live next to the code under `breaker/`.

```sh
go test ./...
```

You should see all tests pass.

## Running the example

There is a small example that simulates a flaky downstream service.

```sh
go run ./examples
```

## Code style and checks

- Keep code idiomatic and simple; prefer clear naming and small functions.
- Ensure new types, exported functions, and behavior changes are covered by unit tests.
- Run `go vet ./...` and `go test ./...` before pushing.

## Pre-commit hooks

We use `pre-commit` to run formatting and lint checks before each commit.

Install `pre-commit` (macOS/Linux):

```sh
pip install pre-commit
```

Enable the hooks in this repo:

```sh
pre-commit install
```

Now the hooks will run automatically on `git commit`.

## Branching and PR guidelines

- Create feature branches from `main`: `feat/<short-name>` or `fix/<short-name>`.
- Keep PRs focused and small; include context in the description.
- Update docs and examples when public APIs change.
- Ensure CI (if configured) and local tests pass before requesting review.

## Filing issues

If you encounter a bug or have a feature request:

- Search existing issues to avoid duplicates
- Provide a reproducible example or steps
- Include version info (Go version, OS) and any logs or error messages

Thanks again for contributing! ⚡
