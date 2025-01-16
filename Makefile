.PHONY: test test_linux

test:
	go test -v ./...
test_linux:
	docker run --rm -it $$(docker build -q .)