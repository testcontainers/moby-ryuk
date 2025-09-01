# -----------
# Build Image
# -----------
FROM golang:1.23-alpine3.22 AS build

WORKDIR /app

# Go build env
ENV CGO_ENABLED=0

# Install source deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source & build
COPY --link . .

# Build binary:
# -a: force rebuild
# -installsuffix cgo: remove cgo support
# -ldflags="-w -s": omits the DWARF symbol table, symbol table and debug information
# -trimpath: remove all file system paths from the compiled executable
# -o /bin/ryuk: output binary to /bin/ryuk
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build \
    -a \
    -installsuffix cgo \
    -ldflags="-w -s" \
    -trimpath \
    -o /bin/ryuk .

# Compress with UPX (trade-off: smaller size vs startup time)
RUN apk add --no-cache upx && upx --best --lzma /bin/ryuk

# -----------------
# Certificates
# -----------------
FROM alpine:3.22 AS certs

RUN apk --no-cache add ca-certificates

# -----------------
# Distributed Image
# -----------------
FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /bin/ryuk /bin/ryuk
CMD ["/bin/ryuk"]
LABEL org.testcontainers.ryuk=true
