# dummy value for linux builds
ARG WINBASE=scratch

FROM --platform=${BUILDPLATFORM} golang:1.18 AS workspace
LABEL builder=true

ENV CGO_ENABLED=0
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

WORKDIR /go/src/github.com/testcontainers/moby-ryuk
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN cd /go/src/github.com/testcontainers/moby-ryuk && go get -d \
 && go vet ./... \
 && go test ./... \
 && if [ "$TARGETARCH" = "arm" ]; then export GOARM="${TARGETVARIANT//v}"; fi; \
    go build -v -a \
    -ldflags "-s -w -extldflags \"-static\"" \
    -o /bin/moby-ryuk main.go; \
    chmod +x /bin/moby-ryuk

FROM ${WINBASE} AS windows
CMD ["/moby-ryuk.exe"]
COPY --from=workspace /bin/moby-ryuk /moby-ryuk.exe

FROM alpine:3.16.1 AS linux
RUN apk --no-cache add ca-certificates
CMD ["/moby-ryuk"]
COPY --from=workspace /bin/moby-ryuk /moby-ryuk
