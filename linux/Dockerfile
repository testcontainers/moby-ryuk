# -----------
# Build Image
# -----------
FROM golang:1.23-alpine3.20 AS build

WORKDIR /app

# Go build env
ENV CGO_ENABLED=0

# Install source deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source & build
COPY --link . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags '-s' -o /bin/ryuk

# -----------------
# Distributed Image
# -----------------
FROM alpine:3.20

RUN apk --no-cache add ca-certificates

COPY --from=build /bin/ryuk /bin/ryuk
CMD ["/bin/ryuk"]
LABEL org.testcontainers.ryuk=true
