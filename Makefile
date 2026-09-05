deps:
	go mod tidy

backends:
	go run ./cmd/server

stop-backends:
	@if [ -f .backend1.pid ]; then kill $$(cat .backend1.pid) 2>/dev/null; rm .backend1.pid; fi
	@if [ -f .backend2.pid ]; then kill $$(cat .backend2.pid) 2>/dev/null; rm .backend2.pid; fi
	@if [ -f .backend3.pid ]; then kill $$(cat .backend3.pid) 2>/dev/null; rm .backend3.pid; fi
	@echo "Backends stopped"

build: clean
	@mkdir -p ./build
	go build -o ./build/l7-proxy ./cmd/proxy

run: build
	./build/l7-proxy

clean:
	rm -f ./build/l7-proxy

clean-all: clean
	rm -f .backend*.pid

all: deps backends build run
