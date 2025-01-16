.PHONY: test test_linux

test:
	go test -v ./...
test_linux:
	docker run --rm -it $$(docker build -q .)
bench:
	go test -run=^$$ -bench=. ./...
bench_linux:
	docker run --rm -it $$(docker build -q .) go test -run=^$$ -bench=. ./...