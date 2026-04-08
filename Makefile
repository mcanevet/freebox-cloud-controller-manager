IMG ?= ghcr.io/mcanevet/freebox-cloud-controller-manager:latest

.PHONY: build
build:
	go build -o bin/freebox-cloud-controller-manager .

.PHONY: test
test:
	go test -v -run '^TestConfig|^TestNode|^TestInstance' ./...

.PHONY: test-unit
test-unit:
	go test -v -race -run '^TestConfig|^TestNode|^TestInstance' ./...

.PHONY: test-integration
test-integration:
	@if ! command -v setup-envtest >/dev/null 2>&1; then \
		echo "Installing setup-envtest..."; \
		go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest; \
	fi
	KUBEBUILDER_ASSETS=$$(setup-envtest use 1.35.0 -p path) go test -v -tags=integration -run '^TestIntegration' ./...

.PHONY: test-all
test-all: test-unit test-integration

.PHONY: test-coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint: fmt vet

.PHONY: docker-build
docker-build:
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push:
	docker push $(IMG)

.PHONY: setup-envtest
setup-envtest:
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
	setup-envtest use 1.35.0
