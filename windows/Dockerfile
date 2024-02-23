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
#FROM mcr.microsoft.com/windows/nanoserver:ltsc2019
#FROM mcr.microsoft.com/windows/nanoserver:ltsc2022

COPY --from=build /bin/ryuk /bin/ryuk
CMD ["/bin/ryuk"]
LABEL org.testcontainers.ryuk=true
