CONFIG_DEBUG ?=-debug
GIT_HASH := $(shell git rev-parse --short HEAD)

config.go:
	go run vendor/github.com/jteeuwen/go-bindata/go-bindata/*.go -o config/config.go -pkg config $(CONFIG_DEBUG) config/

build: config.go | dist
	go build -o dist/k8-spot-daemon-darwin-$(GIT_HASH)

dist:
	[ -d dist ] || mkdir dist

linux: dist/k8-spot-daemon-linux-x86

release: clean routes-debug
	docker build -t k8-spot-daemon:latest .

clean:
	rm -rf dist
	rm -f config/config.go
