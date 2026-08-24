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

# go.mod / go.sum must stay tidy (also gated in CI). Snapshot before tidying and
# compare against that — not the git index — so an unstaged but already-tidy
# dependency change does not fail the check.
tidy-check:
	@cp go.mod go.mod.tidybak && cp go.sum go.sum.tidybak; \
	go mod tidy; \
	rc=0; \
	if ! cmp -s go.mod go.mod.tidybak || ! cmp -s go.sum go.sum.tidybak; then \
		echo "go.mod / go.sum not tidy — run 'go mod tidy' and commit:"; \
		diff go.mod.tidybak go.mod || true; diff go.sum.tidybak go.sum || true; rc=1; \
	fi; \
	mv go.mod.tidybak go.mod; mv go.sum.tidybak go.sum; \
	exit $$rc

# Vulnerability scan (also gated in CI). Installs govulncheck on demand and runs it
# by its install path, since $(go env GOPATH)/bin may not be on PATH.
vuln:
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		go install golang.org/x/vuln/cmd/govulncheck@latest; \
		bin="$$(go env GOBIN)"; "$${bin:-$$(go env GOPATH)/bin}/govulncheck" ./...; \
	fi

fmt:
	gofmt -w .

# Review-thread gate for a PR: make gate PR=123
gate:
	.claude/skills/pr-review-gate/check-review-threads.sh $(PR)

clean:
	rm -rf bin
