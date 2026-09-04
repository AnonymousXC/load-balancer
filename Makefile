deps:
	go mod tidy

backends:
	@echo "Starting backends..."
	@PORT=8082 SID=1 go run ./cmd/server & echo $$! > ./build/.backend1.pid
	@PORT=8083 SID=2 go run ./cmd/server & echo $$! > ./build/.backend2.pid
	@PORT=8084 SID=3 go run ./cmd/server & echo $$! > ./build/.backend3.pid
	@sleep 1
	@echo "Backends started"

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
