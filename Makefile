all: build image push

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o install-kwok --ldflags "-s"

image:
	docker build -t ghcr.io/vadafoss/install-kwok:${TAG} -f Dockerfile.debian .

push:
	docker push ghcr.io/vadafoss/install-kwok:${TAG}