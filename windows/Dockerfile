FROM golang:1.18.4-nanoserver AS workspace
LABEL builder=true

ENV CGO_ENABLED=0
#ENV GO111MODULE=on

WORKDIR /go/src/github.com/testcontainers/moby-ryuk
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN cd /go/src/github.com/testcontainers/moby-ryuk && go get -d \
 && go build \
    -v -a \
    -ldflags "-s -w -extldflags \"-static\"" \
    -o /bin/main main.go

FROM mcr.microsoft.com/windows/nanoserver:ltsc2022
COPY --from=workspace /bin/main /app
CMD ["/app"]
