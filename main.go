package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"gopkg.in/matryer/try.v1"
)

const (
	connectionTimeoutEnv   string = "RYUK_CONNECTION_TIMEOUT"
	portEnv                string = "RYUK_PORT"
	reconnectionTimeoutEnv string = "RYUK_RECONNECTION_TIMEOUT"
	ryukLabel              string = "org.testcontainers.ryuk"
	verboseEnv             string = "RYUK_VERBOSE"
)

var (
	port                int
	connectionTimeout   time.Duration
	reconnectionTimeout time.Duration
	verbose             bool
)

type config struct {
	Port                int
	ConnectionTimeout   time.Duration
	ReconnectionTimeout time.Duration
	Verbose             bool
}

// newConfig parses command line flags and returns a parsed config. config.timeout
// can be set by environment variable, RYUK_CONNECTION_TIMEOUT. If an error occurs
// while parsing RYUK_CONNECTION_TIMEOUT the error is returned.
func newConfig(args []string) (*config, error) {
	cfg := config{
		Port:                8080,
		ConnectionTimeout:   60 * time.Second,
		ReconnectionTimeout: 10 * time.Second,
		Verbose:             false,
	}

	fs := flag.NewFlagSet("ryuk", flag.ExitOnError)
	fs.SetOutput(os.Stdout)

	fs.IntVar(&cfg.Port, "p", 8080, "Deprecated: please use the "+portEnv+" environment variable to set the port to bind at")

	err := fs.Parse(args)
	if err != nil {
		return nil, err
	}

	if timeout, ok := os.LookupEnv(connectionTimeoutEnv); ok {
		parsedTimeout, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse \"%s\": %s", connectionTimeoutEnv, err)
		}

		cfg.ConnectionTimeout = parsedTimeout
	}

	if port, ok := os.LookupEnv(portEnv); ok {
		parsedPort, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("failed to parse \"%s\": %s", portEnv, err)
		}

		cfg.Port = parsedPort
	}

	if timeout, ok := os.LookupEnv(reconnectionTimeoutEnv); ok {
		parsedTimeout, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse \"%s\": %s", reconnectionTimeoutEnv, err)
		}

		cfg.ReconnectionTimeout = parsedTimeout
	}

	if verbose, ok := os.LookupEnv(verboseEnv); ok {
		v, err := strconv.ParseBool(verbose)
		if err != nil {
			return nil, fmt.Errorf("failed to parse \"%s\": %s", verboseEnv, err)
		}

		cfg.Verbose = v
	}

	return &cfg, nil
}

func main() {
	cfg, err := newConfig(os.Args[1:])
	if err != nil {
		panic(err)
	}

	port = cfg.Port
	connectionTimeout = cfg.ConnectionTimeout
	reconnectionTimeout = cfg.ReconnectionTimeout
	verbose = cfg.Verbose

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	cli.NegotiateAPIVersion(context.Background())

	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Println("Pinging Docker...")
	_, err = cli.Ping(pingCtx)
	if err != nil {
		panic(err)
	}

	log.Println("Docker daemon is available!")

	deathNote := sync.Map{}

	connectionAccepted := make(chan net.Addr)
	connectionLost := make(chan net.Addr)

	go processRequests(&deathNote, connectionAccepted, connectionLost)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	waitForPruneCondition(ctx, connectionAccepted, connectionLost)

	dc, dn, dv, di := prune(cli, &deathNote)
	log.Printf("Removed %d container(s), %d network(s), %d volume(s) %d image(s)", dc, dn, dv, di)
}

func processRequests(deathNote *sync.Map, connectionAccepted chan<- net.Addr, connectionLost chan<- net.Addr) {
	log.Printf("Starting on port %d...", port)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}

	log.Println("Started!")
	for {
		conn, err := ln.Accept()
		if err != nil {
			panic(err)
		}

		connectionAccepted <- conn.RemoteAddr()

		go func(conn net.Conn) {
			defer conn.Close()
			defer func() { connectionLost <- conn.RemoteAddr() }()

			reader := bufio.NewReader(conn)
			for {
				message, err := reader.ReadString('\n')

				message = strings.TrimSpace(message)

				if len(message) > 0 {
					query, err := url.ParseQuery(message)

					if err != nil {
						log.Println(err)
						continue
					}

					args := filters.NewArgs()
					for filterType, values := range query {
						for _, value := range values {
							args.Add(filterType, value)
						}
					}
					paramBytes, err := args.MarshalJSON()

					if err != nil {
						log.Println(err)
						continue
					}
					param := string(paramBytes)

					log.Printf("Adding %s", param)

					deathNote.Store(param, true)

					_, _ = conn.Write([]byte("ACK\n"))
				}

				if err != nil {
					log.Println(err)
					break
				}
			}
		}(conn)
	}
}

func waitForPruneCondition(ctx context.Context, connectionAccepted <-chan net.Addr, connectionLost <-chan net.Addr) {
	connectionCount := 0
	never := make(chan time.Time, 1)
	defer close(never)

	handleConnectionAccepted := func(addr net.Addr) {
		log.Printf("New client connected: %s", addr)
		connectionCount++
	}

	select {
	case <-time.After(connectionTimeout):
		panic("Timed out waiting for the first connection")
	case addr := <-connectionAccepted:
		handleConnectionAccepted(addr)
	case <-ctx.Done():
		log.Println("Signal received")
		return
	}

	for {
		var noConnectionTimeout <-chan time.Time
		if connectionCount == 0 {
			noConnectionTimeout = time.After(reconnectionTimeout)
		} else {
			noConnectionTimeout = never
		}

		select {
		case addr := <-connectionAccepted:
			handleConnectionAccepted(addr)
			break
		case addr := <-connectionLost:
			log.Printf("Client disconnected: %s", addr.String())
			connectionCount--
			break
		case <-ctx.Done():
			log.Println("Signal received")
			return
		case <-noConnectionTimeout:
			log.Println("Timed out waiting for re-connection")
			return
		}
	}
}

func prune(cli *client.Client, deathNote *sync.Map) (deletedContainers int, deletedNetworks int, deletedVolumes int, deletedImages int) {
	deletedContainersMap := make(map[string]bool)
	deletedNetworksMap := make(map[string]bool)
	deletedVolumesMap := make(map[string]bool)
	deletedImagesMap := make(map[string]bool)

	deathNote.Range(func(note, _ interface{}) bool {
		param := fmt.Sprint(note)
		if verbose {
			log.Printf("Deleting %s\n", param)
		}

		args, err := filters.FromJSON(param)
		if err != nil {
			log.Println(err)
			return true
		}

		containerListOpts := types.ContainerListOptions{All: true, Filters: args}
		if verbose {
			log.Printf("Listing containers with filter: %#v\n", containerListOpts)
		}

		if containers, err := cli.ContainerList(context.Background(), containerListOpts); err != nil {
			log.Println(err)
		} else {
			containerRemoveOpts := types.ContainerRemoveOptions{RemoveVolumes: true, Force: true}

			for _, container := range containers {
				value, isReaper := container.Labels[ryukLabel]
				if isReaper && value == "true" {
					continue
				}

				if verbose {
					log.Printf("Deleting containers with filter: %#v\n", containerRemoveOpts)
				}
				_ = cli.ContainerRemove(context.Background(), container.ID, containerRemoveOpts)
				deletedContainersMap[container.ID] = true
			}
		}

		_ = try.Do(func(attempt int) (bool, error) {
			if verbose {
				log.Printf("Deleting networks with filter: %#v. (Attempt %d/%d)\n", args, attempt, 10)
			}

			networksPruneReport, err := cli.NetworksPrune(context.Background(), args)
			for _, networkID := range networksPruneReport.NetworksDeleted {
				deletedNetworksMap[networkID] = true
			}
			shouldRetry := attempt < 10
			if err != nil && shouldRetry {
				log.Printf("Network pruning has failed, retrying(%d/%d). The error was: %v", attempt, 10, err)
				time.Sleep(1 * time.Second)
			}
			return shouldRetry, err
		})

		_ = try.Do(func(attempt int) (bool, error) {
			argsClone := args.Clone()

			if verbose {
				log.Printf("Deleting volumes with filter: %#v. (Attempt %d/%d)\n", argsClone, attempt, 10)
			}

			// API version >= v1.42 prunes only anonymous volumes: https://github.com/moby/moby/releases/tag/v23.0.0.
			if serverVersion, err := cli.ServerVersion(context.Background()); err != nil && serverVersion.APIVersion >= "1.42" {
				argsClone.Add("all", "true")
			}

			volumesPruneReport, err := cli.VolumesPrune(context.Background(), argsClone)
			for _, volumeName := range volumesPruneReport.VolumesDeleted {
				deletedVolumesMap[volumeName] = true
			}
			shouldRetry := attempt < 10
			if err != nil && shouldRetry {
				log.Printf("Volumes pruning has failed, retrying(%d/%d). The error was: %v", attempt, 10, err)
				time.Sleep(1 * time.Second)
			}
			return shouldRetry, err
		})

		_ = try.Do(func(attempt int) (bool, error) {
			argsClone := args.Clone()
			argsClone.Add("dangling", "false")

			if verbose {
				log.Printf("Deleting images with filter: %#v. (Attempt %d/%d)\n", argsClone, attempt, 10)
			}

			imagesPruneReport, err := cli.ImagesPrune(context.Background(), argsClone)
			for _, image := range imagesPruneReport.ImagesDeleted {
				if image.Untagged != "" {
					deletedImagesMap[image.Untagged] = true
				}
			}
			shouldRetry := attempt < 10
			if err != nil && shouldRetry {
				log.Printf("Images pruning has failed, retrying(%d/%d). The error was: %v", attempt, 10, err)
				time.Sleep(1 * time.Second)
			}
			return shouldRetry, err
		})

		return true
	})

	deletedContainers = len(deletedContainersMap)
	deletedNetworks = len(deletedNetworksMap)
	deletedVolumes = len(deletedVolumesMap)
	deletedImages = len(deletedImagesMap)
	return
}
