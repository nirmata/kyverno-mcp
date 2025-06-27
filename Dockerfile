# Build the MCP server binary using the Docker's Debian image.
FROM --platform=${BUILDPLATFORM} golang:1.24 AS builder
ARG VERSION
ARG TARGETOS
ARG TARGETARCH
WORKDIR /workspace

# Copy the Go Modules manifests.
COPY go.mod go.mod
COPY go.sum go.sum

# Cache the Go Modules
RUN go mod download

# Copy the Go sources.
COPY cmd/main.go cmd/main.go
COPY pkg/ pkg/

# Build the MCP server binary.
RUN CGO_ENABLED=0 GOFIPS140=latest GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X main.VERSION=${VERSION}" -trimpath -a -o kyverno-mcp cmd/main.go

# Run the MCP server binary using Google's Distroless image.
FROM gcr.io/distroless/static:nonroot
WORKDIR /

# Copy the MCP server binary.
COPY --from=builder /workspace/kyverno-mcp .

# Run the MCP server as a non-root user.
USER 65532:65532
ENTRYPOINT ["/kyverno-mcp"]