GO ?= go
# staticcheck runs through `go run` at a pinned commit (the 0.8.1 release cannot read Go 1.27 export data).
STATICCHECK ?= $(GO) run honnef.co/go/tools/cmd/staticcheck@v0.7.0-0.dev.0.20261009230814-452d5bb86b45
# The race detector needs cgo. CI (Linux) has it; a Windows laptop without a C compiler does not.
RACE := $(if $(filter 1,$(shell $(GO) env CGO_ENABLED)),-race,)

.PHONY: setup build test lint guard acceptance race

setup:
	$(GO) mod download

build:
	$(GO) build ./...

test:
	$(GO) test ./... $(RACE) -count=1

lint:
	$(GO) vet ./...
	$(STATICCHECK) ./...

guard:
	$(GO) run ./scripts/guard

acceptance:
	$(GO) run ./scripts/acceptance

# The race detector needs cgo. Where the host has no C compiler, run it in Linux via Docker.
race:
	docker run --rm -v "$(CURDIR):/src" -w /src golang:1.26 go test -race -count=1 ./...
