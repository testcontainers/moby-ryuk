ARG BASE_IMAGE
# -----------
# Build Image
# -----------
FROM golang:1.24-nanoserver AS build

# Go build env
ENV CGO_ENABLED=0

WORKDIR /app

# Switch to admin user for module operations
USER ContainerAdministrator

# Install source deps
COPY go.mod go.sum ./
RUN go mod download

# Switch back to regular user for security
USER ContainerUser

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
