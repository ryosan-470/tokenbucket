GO = go

test:
	$(GO) test ./... -v -count=1 -race -timeout 60s

.PHONY: benchmark
benchmark:
	$(GO) test -benchtime 10s -bench=. ./benchmark
