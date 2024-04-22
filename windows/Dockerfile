ARG BASE_IMAGE
# -----------
# Build Image
# -----------
FROM golang:1.21-nanoserver as build

# Go build env
ENV CGO_ENABLED=0

WORKDIR /app

# Install source deps
COPY go.mod go.sum ./
RUN go mod download

# Copy source & build
COPY . .
RUN go build -v -ldflags "-s" -o /bin/ryuk

# -----------------
# Distributed Image
# -----------------
FROM ${BASE_IMAGE}

COPY --from=build /bin/ryuk /bin/ryuk
CMD ["/bin/ryuk"]
LABEL org.testcontainers.ryuk=true
HEALTHCHECK --start-period=5s --interval=2s --timeout=2s --retries=10 CMD netstat -ltn | grep -q ":8080"
