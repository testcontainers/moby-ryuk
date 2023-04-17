# Moby Ryuk

This project helps you to remove containers/networks/volumes/images by given filter after specified delay.

# Usage

1. Start it:

        $ RYUK_PORT=8080 ./bin/moby-ryuk
        $ # You can also run it with Docker
        $ docker run -v /var/run/docker.sock:/var/run/docker.sock -e RYUK_PORT=8080 -p 8080:8080 testcontainers/ryuk:0.4.0

1. Connect via TCP:

        $ nc localhost 8080

1. Send some filters:

        label=testing=true&health=unhealthy
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

- `RYUK_CONNECTION_TIMEOUT` - Environment variable that defines the timeout for Ryuk to receive the first connection (default: 60s). Value layout is described in [time.ParseDuration](https://golang.org/pkg/time/#ParseDuration) documentation.
- `RYUK_PORT` - Environment variable that defines the port where Ryuk will be bound to (default: 8080).
- `RYUK_RECONNECTION_TIMEOUT` - Environment variable that defines the timeout for Ryuk to reconnect to Docker (default: 10s). Value layout is described in [time.ParseDuration](https://golang.org/pkg/time/#ParseDuration) documentation.
