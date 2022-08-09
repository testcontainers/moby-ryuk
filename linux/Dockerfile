FROM golang:1.19 AS workspace
LABEL builder=true

ENV CGO_ENABLED=0

WORKDIR /go/src/github.com/testcontainers/moby-ryuk
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN cd /go/src/github.com/testcontainers/moby-ryuk && go get -d \
 && go vet ./... \
 && go test ./... \
 && go build \
    -v -a \
    -ldflags "-s -w -extldflags \"-static\"" \
    -o /bin/moby-ryuk main.go

FROM alpine:3.16.1
RUN apk --no-cache add ca-certificates
COPY --from=workspace /bin/moby-ryuk /moby-ryuk
CMD ["/moby-ryuk"]
