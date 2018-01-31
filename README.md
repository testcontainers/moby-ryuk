# Moby Ryuk

This project helps you to remove containers/networks/volumes by given filter after specified delay.

# Usage

1. Start it:

        $ ./bin/moby-ryuk -p 8080
        $ # You can also run it with Docker
        $ docker run -v /var/run/docker.sock:/var/run/docker.sock -p 8080:8080 quay.io/testcontainers/moby-ryuk

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
        2018/01/15 18:38:52 Removed 1 container(s), 0 network(s), 0 volume(s)
