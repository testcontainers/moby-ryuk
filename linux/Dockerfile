# -----------
# Build Image
# -----------
FROM golang:1.21-alpine3.19 AS build

# Go build env
ENV CGO_ENABLED=0

WORKDIR /app

# Install source deps
COPY go.mod go.sum ./
RUN go mod download

# Copy source & build
COPY . .
RUN go build -v -ldflags '-s' -o /bin/ryuk
RUN chmod +x /bin/ryuk

# -----------------
# Distributed Image
# -----------------
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

COPY --from=build /bin/ryuk /bin/ryuk
CMD ["/bin/ryuk"]
LABEL org.testcontainers.ryuk=true
