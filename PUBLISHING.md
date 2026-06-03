# Publishing & External Consumption

## Current State

The three modules use `replace` directives for local development:

```
agent/go.mod:  replace github.com/chinudotdev/pi-go/ai => ../ai
sdk/go.mod:    replace github.com/chinudotdev/pi-go/agent => ../agent
               replace github.com/chinudotdev/pi-go/ai => ../ai
```

These are **required** — even with `go.work`, the Go toolchain needs them to
resolve `v0.0.0` module versions without hitting the remote registry.

## How to Publish

### Step 1: Tag with module prefixes

```bash
git tag ai/v0.1.0
git tag agent/v0.1.0
git tag sdk/v0.1.0
git push origin --tags
```

Go's module mirror recognizes submodules via `go.mod` at subdirectories
and resolves `github.com/chinudotdev/pi-go/sdk@v0.1.0` correctly.

### Step 2: Consumers import normally

```go
// Their go.mod:
require github.com/chinudotdev/pi-go/sdk v0.1.0

// Go automatically resolves:
// sdk v0.1.0 → agent v0.1.0 → ai v0.1.0
```

No `replace` directives needed on the consumer side — the `replace` in our
`go.mod` files are **ignored** by consumers (Go only applies `replace` from
the main module, not dependencies).

### Step 3: Bump versions

```bash
# Patch bump (bug fix)
git tag ai/v0.1.1 agent/v0.1.1 sdk/v0.1.1

# Minor bump (new feature)
git tag ai/v0.2.0 agent/v0.2.0 sdk/v0.2.0
```

## For Local Development (current workflow)

No changes needed. `go.work` + `replace` directives work together:

```bash
# Build and test everything
go build ./...
go test ./...

# Or just one module
cd sdk && go test ./...
```

## For Personal Use Before Publishing

If you want to use the SDK in another project before publishing tags:

### Option A: go.work (if both repos are on disk)

```bash
# In your app's go.work:
go 1.24.5

use (
    .                                    # your app
    /path/to/pi-go/sdk                   # SDK
    /path/to/pi-go/agent                 # agent (needed by SDK)
    /path/to/pi-go/ai                    # ai (needed by agent)
)
```

### Option B: replace in your app's go.mod

```go
// Your app's go.mod:
require github.com/chinudotdev/pi-go/sdk v0.0.0-local

replace (
    github.com/chinudotdev/pi-go/sdk    => /path/to/pi-go/sdk
    github.com/chinudotdev/pi-go/agent  => /path/to/pi-go/agent
    github.com/chinudotdev/pi-go/ai     => /path/to/pi-go/ai
)
```

### Option C: GOPROXY=off (skip module proxy)

```bash
GOPROXY=off go get github.com/chinudotdev/pi-go/sdk@latest
# Requires the repo to be in your GOPATH or a remote that Go can clone
```
