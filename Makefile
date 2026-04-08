IMG ?= ghcr.io/mcanevet/freebox-cloud-controller-manager:latest

.PHONY: build
build:
	go build -o bin/freebox-cloud-controller-manager .

.PHONY: test
test:
	go test ./...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: docker-build
docker-build:
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push:
	docker push $(IMG)
