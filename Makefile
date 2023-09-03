.PHONY: test 
test:
	go test ./... -v

.PHONY: lint 
lint:
	gofmt -w .