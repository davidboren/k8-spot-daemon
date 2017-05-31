CONFIG_DEBUG ?=-debug

config.go:
	go run vendor/github.com/jteeuwen/go-bindata/go-bindata/*.go -o config/config.go -pkg config $(CONFIG_DEBUG) config/

build: config.go | dist
	go build -o dist/k8-spot-daemon

dist:
	[ -d dist ] || mkdir dist

dist/k8-spot-daemon-linux-x86: config.go dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/k8-spot-daemon-linux-x86


linux: dist/k8-spot-daemon-linux-x86

release: linux
	docker build -t dboren/k8-spot-daemon:latest ./
	docker push dboren/k8-spot-daemon:latest

clean:
	rm -rf dist
	rm -f config/config.go
