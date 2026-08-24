.PHONY: build test lint fmt vet tidy-check vuln gate clean

BIN := bin/safe_cli

build:
	go build -o $(BIN) ./cmd/safe_cli

test:
	go test -race ./...

# gofmt is not part of `go vet`; check it explicitly so drift fails, not reformats.
lint: vet tidy-check
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "(golangci-lint not installed locally; CI runs it)"; \
	fi

vet:
	go vet ./...

# go.mod / go.sum must stay tidy (also gated in CI).
tidy-check:
	@go mod tidy; \
	if ! git diff --quiet go.mod go.sum; then \
		echo "go.mod / go.sum not tidy — run 'go mod tidy' and commit:"; \
		git --no-pager diff go.mod go.sum; exit 1; \
	fi

# Vulnerability scan (also gated in CI). Installs govulncheck on demand.
vuln:
	@command -v govulncheck >/dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

fmt:
	gofmt -w .

# Review-thread gate for a PR: make gate PR=123
gate:
	.claude/skills/pr-review-gate/check-review-threads.sh $(PR)

clean:
	rm -rf bin
