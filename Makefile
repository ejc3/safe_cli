.PHONY: build test lint fmt vet gate clean

BIN := bin/safe_cli

build:
	go build -o $(BIN) ./cmd/safe_cli

test:
	go test -race ./...

# gofmt is not part of `go vet`; check it explicitly so drift fails, not reformats.
lint: vet
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

fmt:
	gofmt -w .

# Review-thread gate for a PR: make gate PR=123
gate:
	.claude/skills/pr-review-gate/check-review-threads.sh $(PR)

clean:
	rm -rf bin
