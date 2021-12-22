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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"gopkg.in/matryer/try.v1"
)

var port = flag.Int("p", 8080, "Port to bind at")

func main() {
	flag.Parse()
	log.Println("Pinging Docker...")

	cli, err := client.NewEnvClient()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err = cli.Ping(ctx)
	}

	if err != nil {
		panic(err)
	}

	log.Println("Docker daemon is available!")

	deathNote := make(map[string]bool)

	firstConnected := make(chan bool, 1)
	var wg sync.WaitGroup

	go processRequests(deathNote, firstConnected, &wg)

	select {
	case <-time.After(1 * time.Minute):
		panic("Timed out waiting for the first connection")
	case <-firstConnected:
	}
	log.Println("Received the first connection")

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		wg.Wait()
		log.Println("Timed out waiting for re-connection")
		signals <- syscall.SIGTERM
	}()

	<-signals

	prune(cli, deathNote)
	os.Exit(0)
}

func processRequests(deathNote map[string]bool, firstConnected chan<- bool, wg *sync.WaitGroup) {
	var once sync.Once

	log.Printf("Starting on port %d...", *port)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))

	if err != nil {
		panic(err)
	}
	log.Println("Started!")
	for {
		conn, err := ln.Accept()
		if err != nil {
			panic(err)
		}
		log.Printf("New client connected: %s\n", conn.RemoteAddr().String())
		go func() {
			wg.Add(1)
			defer wg.Done()
			once.Do(func() {
				firstConnected <- true
			})
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
					param, err := filters.ToParam(args)

					if err != nil {
						log.Println(err)
						continue
					}

					log.Printf("Adding %s\n", param)

					deathNote[param] = true

					conn.Write([]byte("ACK\n"))
				}

				if err != nil {
					log.Println(err)
					break
				}
			}
			log.Printf("Client disconnected: %s\n", conn.RemoteAddr().String())
			conn.Close()

			time.Sleep(10 * time.Second)
		}()
	}
}

func prune(cli *client.Client, deathNote map[string]bool) {
	deletedContainers := make(map[string]bool)
	deletedNetworks := make(map[string]bool)
	deletedVolumes := make(map[string]bool)
	deletedImages := make(map[string]bool)

	for param := range deathNote {
		log.Printf("Deleting %s\n", param)

		args, err := filters.FromParam(param)
		if err != nil {
			log.Println(err)
			continue
		}

		if containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true, Filters: args}); err != nil {
			log.Println(err)
		} else {
			for _, container := range containers {
				cli.ContainerRemove(context.Background(), container.ID, types.ContainerRemoveOptions{RemoveVolumes: true, Force: true})
				deletedContainers[container.ID] = true
			}
		}

		try.Do(func(attempt int) (bool, error) {
			networksPruneReport, err := cli.NetworksPrune(context.Background(), args)
			for _, networkID := range networksPruneReport.NetworksDeleted {
				deletedNetworks[networkID] = true
			}
			shouldRetry := attempt < 10
			if err != nil && shouldRetry {
				log.Printf("Network pruning has failed, retrying(%d/%d). The error was: %v", attempt, 10, err)
				time.Sleep(1 * time.Second)
			}
			return shouldRetry, err
		})

		try.Do(func(attempt int) (bool, error) {
			volumesPruneReport, err := cli.VolumesPrune(context.Background(), args)
			for _, volumeName := range volumesPruneReport.VolumesDeleted {
				deletedVolumes[volumeName] = true
			}
			shouldRetry := attempt < 10
			if err != nil && shouldRetry {
				log.Printf("Volumes pruning has failed, retrying(%d/%d). The error was: %v", attempt, 10, err)
				time.Sleep(1 * time.Second)
			}
			return shouldRetry, err
		})

		try.Do(func(attempt int) (bool, error) {
			args.Add("dangling", "false")
			imagesPruneReport, err := cli.ImagesPrune(context.Background(), args)
			for _, image := range imagesPruneReport.ImagesDeleted {
				deletedImages[image.Deleted] = true
			}
			shouldRetry := attempt < 10
			if err != nil && shouldRetry {
				log.Printf("Images pruning has failed, retrying(%d/%d). The error was: %v", attempt, 10, err)
				time.Sleep(1 * time.Second)
			}
			return shouldRetry, err
		})
	}

	log.Printf("Removed %d container(s), %d network(s), %d volume(s) %d image(s)", len(deletedContainers), len(deletedNetworks), len(deletedVolumes), len(deletedImages))
}
