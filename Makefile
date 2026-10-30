PKG     := ./...
COVER   := coverage.out
VERSION := $(shell git describe --tags --dirty --always 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
FUZZTIME ?= 30s

.PHONY: build test test-live fuzz lint vet cover clean ci ci-test ci-fuzz ci-build ci-tidy ci-lint

# The platforms CI cross-compiles for, as GOOS/GOARCH[/GOARM].
PLATFORMS := linux/amd64 linux/arm64 linux/arm/7 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64

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

# Everything CI runs, in the same way, so a green `make ci` means a green
# pipeline. Each job is also its own target.
ci: ci-tidy ci-lint ci-build ci-test ci-fuzz
	@echo "== ci: all jobs passed"

ci-test:
	@echo "== ci: test"
	go test -race ./...

ci-fuzz:
	@echo "== ci: fuzz (smoke)"
	$(MAKE) --no-print-directory fuzz FUZZTIME=15s

ci-build:
	@echo "== ci: build"
	@for p in $(PLATFORMS); do \
		goos=$${p%%/*}; rest=$${p#*/}; goarch=$${rest%%/*}; goarm=$$(echo $$rest | cut -s -d/ -f2); \
		echo "-- $$p"; \
		GOOS=$$goos GOARCH=$$goarch GOARM=$$goarm CGO_ENABLED=0 \
			go build -ldflags "-s -w -X main.version=$(VERSION)" -o /dev/null ./cmd/sbx || exit 1; \
		GOOS=$$goos GOARCH=$$goarch GOARM=$$goarm CGO_ENABLED=0 go vet ./... || exit 1; \
	done

ci-tidy:
	@echo "== ci: go.mod tidy"
	go mod tidy -diff

ci-lint:
	@echo "== ci: lint"
	@command -v golangci-lint >/dev/null || (echo "golangci-lint not installed: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run --timeout 5m
