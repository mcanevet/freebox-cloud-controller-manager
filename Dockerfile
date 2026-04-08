FROM golang:1.25 AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o freebox-cloud-controller-manager .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/freebox-cloud-controller-manager /freebox-cloud-controller-manager
ENTRYPOINT ["/freebox-cloud-controller-manager"]
