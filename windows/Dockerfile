ARG BASE_IMAGE
# -----------
# Build Image
# -----------
FROM golang:1.23-nanoserver AS build

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
# Build binary:
# -a: force rebuild
# -installsuffix cgo: remove cgo support
# -ldflags="-w -s": omits the DWARF symbol table, symbol table and debug information
# -trimpath: remove all file system paths from the compiled executable
# -o /bin/ryuk: output binary to /bin/ryuk
RUN go build -a -installsuffix cgo -ldflags="-w -s" -trimpath -o /bin/ryuk .

# Do not optimise with UPX on Windows, as nanoserver does not have PowerShell to install it

# -----------------
# Distributed Image
# -----------------
FROM ${BASE_IMAGE}

COPY --from=build /bin/ryuk /bin/ryuk
CMD ["/bin/ryuk"]
LABEL org.testcontainers.ryuk=true
