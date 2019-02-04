FROM golang:1.9 as workspace
WORKDIR /go/src/github.com/testcontainers/moby-ryuk
COPY glide.lock glide.yaml Makefile ./
RUN make bootstrap
COPY . .
RUN make build

FROM alpine:3.9
RUN apk --no-cache add ca-certificates
COPY --from=workspace /go/src/github.com/testcontainers/moby-ryuk/bin/moby-ryuk /app
CMD ["/app"]
