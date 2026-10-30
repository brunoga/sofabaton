PKG     := ./...
COVER   := coverage.out
VERSION := $(shell git describe --tags --dirty --always 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
FUZZTIME ?= 30s

.PHONY: build test test-live fuzz lint vet cover clean

build:
	go build $(LDFLAGS) -o sbx ./cmd/sbx

test:
	go test -race $(PKG)

# Against a real hub; touches only ZZ-test-* entities. See CONTRIBUTING.md.
test-live:
	@test -n "$(SOFABATON_HUB)" || (echo "set SOFABATON_HUB=<hub ip>" && exit 1)
	go test -tags live -race -count=1 -timeout 30m $(PKG)

# Runs every fuzz target for FUZZTIME each.
fuzz:
	@for pkg in $$(go list $(PKG)); do \
		for f in $$(go test -list '^Fuzz' $$pkg | grep '^Fuzz'); do \
			echo "== $$pkg $$f"; \
			go test -run '^$$' -fuzz "^$$f$$" -fuzztime $(FUZZTIME) $$pkg || exit 1; \
		done; \
	done

vet:
	go vet $(PKG)

lint:
	golangci-lint run $(PKG)

cover:
	go test -race -coverprofile=$(COVER) $(PKG)
	go tool cover -html=$(COVER) -o coverage.html

clean:
	rm -f sbx $(COVER) coverage.html
