# Moby Massacrer

This project helps you to remove containers/networks/volumes by given filter after specified delay.

# Usage

1. Start it:

        $ ./bin/moby-massacrer -p 8080

1. Submit cleanup request:

        $ curl -d "label=testing=true" -d "health=unhealthy" http://localhost:8080/schedule?delay=1h

1. Realize that 1 hour is too long for the demo and change it to 5 seconds:

        $ curl -d "label=testing=true" -d "health=unhealthy" http://localhost:8080/schedule?delay=5s

1. See containers/networks/volumes deleted after 5s
