package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

var port = flag.Int("p", 8080, "Port to bind at")

func main() {
	flag.Parse()
	log.Printf("Starting on port %d...", *port)

	schedule := make(map[string]time.Time)

	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	go runMassacre(&schedule, cli)

	mux := http.NewServeMux()
	mux.HandleFunc("/schedule", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Only 'POST' method is allowed", http.StatusMethodNotAllowed)
			return
		}

		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))

		if err != nil {
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
			return
		}

		if mediaType != "application/x-www-form-urlencoded" {
			http.Error(w, "Only 'application/x-www-form-urlencoded' content type is allowed", http.StatusUnsupportedMediaType)
			return
		}

		err = r.ParseForm()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		args := filters.NewArgs()

		for filterType, values := range r.PostForm {
			for _, value := range values {
				args.Add(filterType, value)
			}
		}

		if args.Len() <= 0 {
			http.Error(w, "Empty filters", http.StatusBadRequest)
			return
		}

		delay, err := time.ParseDuration(r.URL.Query().Get("delay"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		param, err := filters.ToParam(args)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		at := time.Now().Add(delay)
		log.Printf("Scheduling %s at %s", param, at)
		schedule[param] = at

		w.WriteHeader(http.StatusAccepted)
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), mux))
}

func runMassacre(schedule *map[string]time.Time, cli *client.Client) {
	var now time.Time
	for {
		now = time.Now()
		oldSchedule := *schedule

		*schedule = make(map[string]time.Time)

		deletedContainers := make(map[string]bool)
		deletedNetworks := make(map[string]bool)
		deletedVolumes := make(map[string]bool)

		for param, after := range oldSchedule {
			if now.Before(after) {
				(*schedule)[param] = after
				continue
			}
			log.Printf("Deleting %s after %s\n", param, after)

			args, err := filters.FromParam(param)
			if err != nil {
				log.Println(err)
				continue
			}

			containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true, Filters: args})
			if err != nil {
				log.Println(err)
			} else {
				for _, container := range containers {
					cli.ContainerRemove(context.Background(), container.ID, types.ContainerRemoveOptions{RemoveVolumes: true, Force: true})
					deletedContainers[container.ID] = true
				}
			}

			networksPruneReport, err := cli.NetworksPrune(context.Background(), args)
			for _, networkID := range networksPruneReport.NetworksDeleted {
				deletedNetworks[networkID] = true
			}

			volumesPruneReport, err := cli.VolumesPrune(context.Background(), args)
			for _, volumeName := range volumesPruneReport.VolumesDeleted {
				deletedVolumes[volumeName] = true
			}
		}
		if len(deletedContainers)+len(deletedNetworks)+len(deletedVolumes) <= 0 {
			time.Sleep(time.Second)
		} else {
			log.Printf("Removed %d container(s), %d network(s), %d volume(s)", len(deletedContainers), len(deletedNetworks), len(deletedVolumes))
		}

	}
}
