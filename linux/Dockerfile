# -----------
# Build Image
# -----------
FROM golang:1.21-alpine3.19 AS build

WORKDIR /app

# Go build env
ENV CGO_ENABLED=0 \
    GOCACHE=/go-cache \
    GOMODCACHE=/gomod-cache

# Install source deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/gomod-cache \
    go mod download

# Copy source & build
COPY . .
RUN --mount=type=cache,target=/gomod-cache \
    --mount=type=cache,target=/go-cache \
    go build -v -ldflags '-s' -o /bin/ryuk

# -----------------
# Distributed Image
# -----------------
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

COPY --from=build /bin/ryuk /bin/ryuk
CMD ["/bin/ryuk"]
LABEL org.testcontainers.ryuk=true
