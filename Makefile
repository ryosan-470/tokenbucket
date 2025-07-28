GO = go

test:
	$(GO) test ./... -v -count=1 -race -timeout 60s
