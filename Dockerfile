FROM golang:1.9 as workspace
WORKDIR /go/src/github.com/bsideup/moby-massacrer
COPY glide.lock glide.yaml Makefile ./
RUN make bootstrap
COPY . .
RUN make build

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=workspace /go/src/github.com/bsideup/moby-massacrer/bin/moby-massacrer /app
CMD ["/app"]
