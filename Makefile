deps:
	go mod tidy

backends:
	python -m http.server 8081 &
	python -m http.server 8082 &
	python -m http.server 8083 &

stop-backends: 
	ps aux | grep "http.server" &
	kill 12345 12346 12347

build:
	go build -o l7-proxy ./cmd/proxy

run: backends build
	./l7-proxy

clean:
	rm -f l7-proxy

bench:
	hey -z 30s -c 100 http://localhost:8080/

all: deps backends build run
