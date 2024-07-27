# Moby Ryuk

This project helps you to remove containers, networks, volumes and images by given filter after specified delay.

## Building

To build the binary only run:

```shell
go build
```

To build the Linux docker container as the latest tag:

```shell
docker build -f linux/Dockerfile -t testcontainers/ryuk:latest .
```

## Usage

1. Start it:

        RYUK_PORT=8080 ./bin/moby-ryuk
        # You can also run it with Docker
        docker run -v /var/run/docker.sock:/var/run/docker.sock -e RYUK_PORT=8080 -p 8080:8080 testcontainers/ryuk:0.6.0

1. Connect via TCP:

        nc localhost 8080

1. Send some filters:

        label=testing=true&label=testing.sessionid=mysession
        ACK
        label=something
        ACK

1. Close the connection

1. Send more filters with "one-off" style:

        printf "label=something_else" | nc localhost 8080

1. See containers/networks/volumes deleted after 10s:

        2018/01/15 18:38:52 Timed out waiting for connection
        2018/01/15 18:38:52 Deleting {"label":{"something":true}}
        2018/01/15 18:38:52 Deleting {"label":{"something_else":true}}
        2018/01/15 18:38:52 Deleting {"health":{"unhealthy":true},"label":{"testing=true":true}}
        2018/01/15 18:38:52 Removed 1 container(s), 0 network(s), 0 volume(s), 0 image(s)

## Ryuk configuration

The following environment variables can be configured to change the behaviour:

| Environment Variable | Default | Format | Description  |
| - | - | - | - |
| `RYUK_CONNECTION_TIMEOUT` | `60s` | [Duration](https://golang.org/pkg/time/#ParseDuration) | The duration without receiving any connections which will trigger a shutdown |
| `RYUK_PORT` | `8080` | `uint16` | The port to listen on for connections |
| `RYUK_RECONNECTION_TIMEOUT` | `10s` |  [Duration](https://golang.org/pkg/time/#ParseDuration) | The duration after the last connection closes which will trigger resource clean up and shutdown |
| `RYUK_REQUEST_TIMEOUT` | `10s` | [Duration](https://golang.org/pkg/time/#ParseDuration) | The timeout for any Docker requests |
| `RYUK_REMOVE_RETRIES` | `10` | `int` | The number of times to retry removing a resource |
| `RYUK_RETRY_OFFSET` | `-1s` | [Duration](https://golang.org/pkg/time/#ParseDuration) | The offset added to the start time of the prune pass that is used as the minimum resource creation time. Any resource created after this calculated time will trigger a retry to ensure in use resources are not removed |
| `RYUK_VERBOSE` | `false` | `bool` | Whether to enable verbose aka debug logging |
| `RYUK_SHUTDOWN_TIMEOUT` | `10m` | [Duration](https://golang.org/pkg/time/#ParseDuration) | The duration after shutdown has been requested when the remaining connections are ignored and prune checks start |
