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

To start it using the default settings:

```shell
docker run -v /var/run/docker.sock:/var/run/docker.sock -p 8080:8080 testcontainers/ryuk:latest
```

If you want to test local changes with the default settings:

```shell
go run .
```

You can then simulate a connection from testcontainer container using:

```shell
nc -N localhost 8080 << EOF
label=testing=true&label=testing.sessionid=mysession
label=something
EOF
```

You can send additional session information for monitoring using:

```shell
printf "label=something_else" | nc -N localhost 8080
```

In the ryuk window you'll see containers/networks/volumes deleted after 10s

```log
time=2024-09-30T19:42:30.000+01:00 level=INFO msg=starting connection_timeout=1m0s reconnection_timeout=10s request_timeout=10s shutdown_timeout=10m0s remove_retries=10 retry_offset=-1s port=8080 verbose=false
time=2024-09-30T19:42:30.001+01:00 level=INFO msg="Started"
time=2024-09-30T19:42:30.001+01:00 level=INFO msg="client processing started"
time=2024-09-30T19:42:38.002+01:00 level=INFO msg="client connected" address=127.0.0.1:56432 clients=1
time=2024-09-30T19:42:38.002+01:00 level=INFO msg="adding filter" type=label values="[testing=true testing.sessionid=mysession]"
time=2024-09-30T19:42:38.002+01:00 level=INFO msg="adding filter" type=label values=[something]
time=2024-09-30T19:42:38.002+01:00 level=INFO msg="client disconnected" address=127.0.0.1:56432 clients=0
time=2024-09-30T19:42:42.047+01:00 level=INFO msg="adding filter" type=label values=[something_else]
time=2024-09-30T19:42:42.047+01:00 level=INFO msg="client connected" address=127.0.0.1:56434 clients=1
time=2024-09-30T19:42:42.047+01:00 level=INFO msg="client disconnected" address=127.0.0.1:56434 clients=0
time=2024-09-30T19:42:52.051+01:00 level=INFO msg="prune check" clients=0
time=2024-09-30T19:42:52.216+01:00 level=INFO msg="client processing stopped"
time=2024-09-30T19:42:52.216+01:00 level=INFO msg=removed containers=0 networks=0 volumes=0 images=0
time=2024-09-30T19:42:52.216+01:00 level=INFO msg=done
```

## Ryuk configuration

The following environment variables can be configured to change the behaviour:

| Environment Variable        | Default | Format  | Description  |
| --------------------------- | ------- | ------- | ------------ |
| `RYUK_CONNECTION_TIMEOUT`   | `60s`   | [Duration](https://golang.org/pkg/time/#ParseDuration) | The duration without receiving any connections which will trigger a shutdown |
| `RYUK_PORT`                 | `8080`  | `uint16` | The port to listen on for connections |
| `RYUK_RECONNECTION_TIMEOUT` | `10s`   | [Duration](https://golang.org/pkg/time/#ParseDuration) | The duration after the last connection closes which will trigger resource clean up and shutdown |
| `RYUK_REQUEST_TIMEOUT`      | `10s`   | [Duration](https://golang.org/pkg/time/#ParseDuration) | The timeout for any Docker requests |
| `RYUK_REMOVE_RETRIES`       | `10`    | `int` | The number of times to retry removing a resource |
| `RYUK_RETRY_OFFSET`         | `-1s`   | [Duration](https://golang.org/pkg/time/#ParseDuration) | The offset added to the start time of the prune pass that is used as the minimum resource creation time. Any resource created after this calculated time will trigger a retry to ensure in use resources are not removed |
| `RYUK_VERBOSE`              | `false` | `bool` | Whether to enable verbose aka debug logging |
| `RYUK_SHUTDOWN_TIMEOUT`     | `10m`   | [Duration](https://golang.org/pkg/time/#ParseDuration) | The duration after shutdown has been requested when the remaining connections are ignored and prune checks start |
