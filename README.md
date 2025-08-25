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
time=2024-09-30T19:42:30.000+01:00 level=INFO msg=starting connection_timeout=1m0s reconnection_timeout=10s request_timeout=10s shutdown_timeout=10m0s remove_retries=10 retry_offset=-1s changes_retry_interval=1s port=8080 verbose=false
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

| Environment Variable          | Default | Format  | Description  |
| ----------------------------- | ------- | ------- | ------------ |
| `RYUK_CONNECTION_TIMEOUT`     | `60s`   | [Duration](https://golang.org/pkg/time/#ParseDuration) | The duration without receiving any connections which will trigger a shutdown |
| `RYUK_PORT`                   | `8080`  | `uint16` | The port to listen on for connections |
| `RYUK_RECONNECTION_TIMEOUT`   | `10s`   | [Duration](https://golang.org/pkg/time/#ParseDuration) | The duration after the last connection closes which will trigger resource clean up and shutdown |
| `RYUK_REQUEST_TIMEOUT`        | `10s`   | [Duration](https://golang.org/pkg/time/#ParseDuration) | The timeout for any Docker requests |
| `RYUK_REMOVE_RETRIES`         | `10`    | `int` | The number of times to retry removing a resource |
| `RYUK_RETRY_OFFSET`           | `-1s`   | [Duration](https://golang.org/pkg/time/#ParseDuration) | The offset added to the start time of the prune pass that is used as the minimum resource creation time. Any resource created after this calculated time will trigger a retry to ensure in use resources are not removed |
| `RYUK_CHANGES_RETRY_INTERVAL` | `1s`    | [Duration](https://golang.org/pkg/time/#ParseDuration) | The internal between retries if resource changes (containers, networks, images, and volumes) are detected while pruning |
| `RYUK_VERBOSE`                | `false` | `bool` | Whether to enable verbose aka debug logging |
| `RYUK_SHUTDOWN_TIMEOUT`       | `10m`   | [Duration](https://golang.org/pkg/time/#ParseDuration) | The duration after shutdown has been requested when the remaining connections are ignored and prune checks start |

## Integrating Ryuk in the Testcontainers Libraries

The Testcontainers libraries can be configured to use Ryuk to remove resources after a test session has completed.

- Identify test session semantics for the Testcontainers library. For example, a test session could be a single test method, a test class, or a test suite. As reference, please consider taking a look at Go's implementation [here](https://golang.testcontainers.org/features/test_session_semantics/). This unique identifier for the test session semantic, is referenced as `SESSION_ID` from now on.
As an implementation hint, consider how an atomic user interaction with the intent of running tests should generally lead to one single session (i.e. run tests from within IDE).
- Use the above configuration to start Ryuk as a special container within the library. For that, read the above environment variables and/or from the Testcontainers properties file, which is located in the home directory of the user. Regarding precedence, the environment variables must have higher precedence than the properties file.
    - Define Ryuk as a container with privileged access.
    - Define a wait strategy for the listening port, defined by the `RYUK_PORT` environment variable. This is necessary to ensure that Ryuk is ready to receive messages from the Testcontainers library.
    - Bind the Docker socket to the Ryuk container, so that it can communicate with the Docker daemon. This is necessary to be able to create and remove resources.
        - E.g. `${RESOLVED_DOCKER_SOCKET}:/var/run/docker.sock`, where `${RESOLVED_DOCKER_SOCKET}` is the path to the Docker socket discovered by the Testcontainers library.
    - Optionally, name the container using the `SESSION_ID` to make it easier to identify the container in the Docker daemon.
    - Expose the port defined by the `RYUK_PORT` environment variable, so that the Testcontainers library can send messages to Ryuk.
    - Add a special label to the Ryuk container in order to avoid removing it by mistake.
        - E.g. `org.testcontainers.reaper=true`, `org.testcontainers.ryuk=true`, etc.
        - If you already use a specific label for reaping resources, please remember to remove it from the Ryuk container for the same reason.
    - Ryuk should run in the default bridge network of the Docker runtime.
- Every time a Docker resource is created in the Testcontainers library, Ryuk must be informed about it. This can be done by sending a message to Ryuk with the Docker labels of the resource, as a set of key-value pairs. In general, it's a good practice to always send the same set of labels for all the resources, including the above `SESSION_ID`, so that Ryuk can consistenly identify and remove the created resources after the test session has completed.
- Use a TCP connection to send Ryuk the message. The connection must be established to the address of the Ryuk container and the port specified in the `RYUK_PORT` environment variable.
    - An example: `localhost:8080`. Please use the Tescontainers library to get the address of the container, not hardcoding `localhost` or any other address.
- The message sent to Ryuk must be a string, with the Docker filter format, as follows:
    - Each label must be represented as a key-value pair, separated by an equal sign (`=`).
    - Labels must be separated among them by an ampersand (`&`).
    - The message must be terminated by a newline character (`\n`).
    - An example: `label=testing=true&label=testing.sessionid=mysession\n`.
- Once received by Ryuk, the message is processed and stored as a Docker filter.
- Ryuk responds with an acknowledgment message, with the constant value of `ACK\n`, which can be used to check if the message was successfully processed, completing the handshake.
- Whenever a resource is removed by the Testcontainers library, send a termination signal to Ryuk using a TCP connection in the same way as seen above; this way Ryuk can identify the test session is about to finish and start the cleanup process. Ryuk uses `RYUK_CONNECTION_TIMEOUT` and `RYUK_RECONNECTION_TIMEOUT` to determine when to start the cleanup process.
