package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

//nolint:gochecknoglobals // Reusable options are fine as globals.
var (
	// errChangesDetected is returned when changes are detected.
	errChangesDetected = errors.New("changes detected")

	// containerRemoveOptions are the options we use to remove a container.
	containerRemoveOptions = container.RemoveOptions{RemoveVolumes: true, Force: true}

	// imageRemoveOptions are the options we use to remove an image.
	imageRemoveOptions = image.RemoveOptions{PruneChildren: true}

	// volumeRemoveForce is the force option we use to remove a volume.
	volumeRemoveForce = true

	// ackResponse is the response we send to the client to acknowledge a filter.
	ackResponse = []byte("ACK\n")
)

// reaper listens for connections and prunes resources based on the filters received
// once a prune condition is met.
type reaper struct {
	client       dockerClient
	listener     net.Listener
	cfg          *config
	connected    chan string
	disconnected chan string
	shutdown     chan struct{}
	filters      map[string]filters.Args
	logger       *slog.Logger
	mtx          sync.Mutex
}

// reaperOption is a function that sets an option on a reaper.
type reaperOption func(*reaper) error

// withConfig returns a reaperOption that sets the configuration.
// Default: loaded from the environment.
func withConfig(cfg config) reaperOption {
	return func(r *reaper) error {
		r.cfg = &cfg
		return nil
	}
}

// withLogger returns a reaperOption that sets the logger.
// If specified the log level will not be changed by the
// configuration, so should be set to the desired level.
// Default: A text handler to stdout with the log level
// set by the configuration.
func withLogger(logger *slog.Logger) reaperOption {
	return func(r *reaper) error {
		r.logger = logger
		return nil
	}
}

// withClient returns a reaperOption that sets the Docker client.
// Default: A docker client created with options from the environment.
func withClient(client dockerClient) reaperOption {
	return func(r *reaper) error {
		r.client = client
		return nil
	}
}

// newReaper creates a new reaper with the specified options.
// Default options are used if not specified, see the individual
// options for details.
func newReaper(ctx context.Context, options ...reaperOption) (*reaper, error) {
	logLevel := &slog.LevelVar{}
	r := &reaper{
		filters:      make(map[string]filters.Args),
		connected:    make(chan string), // Must be unbuffered to ensure correct behaviour.
		disconnected: make(chan string),
		shutdown:     make(chan struct{}),
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})),
	}

	for _, option := range options {
		if err := option(r); err != nil {
			return nil, fmt.Errorf("option: %w", err)
		}
	}

	var err error
	if r.client == nil {
		// Default client configured from the environment.
		if r.client, err = client.NewClientWithOpts(client.FromEnv); err != nil {
			return nil, fmt.Errorf("new client: %w", err)
		}
	}

	r.client.NegotiateAPIVersion(ctx)

	if r.cfg == nil {
		// Default configuration loaded from the environment.
		if r.cfg, err = loadConfig(); err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}

	if r.cfg.Verbose {
		logLevel.Set(slog.LevelDebug)
	}

	pingCtx, cancel := context.WithTimeout(ctx, r.cfg.RequestTimeout)
	defer cancel()

	if _, err = r.client.Ping(pingCtx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}

	r.logger.LogAttrs(ctx, slog.LevelInfo, "starting", r.cfg.LogAttrs()...)
	if r.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", r.cfg.Port)); err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	// This log message, in uppercase, is in use in different Testcontainers libraries,
	// so it is important to keep it as is to not break the current behavior of the libraries.
	r.logger.Info("Started", fieldAddress, r.listener.Addr().String())

	return r, nil
}

// run starts the reaper which prunes resources when:
//   - Signalled by the context
//   - No connections are received within the connection timeout
//   - A connection is received and no further connections are received within the reconnection timeout
func (r *reaper) run(ctx context.Context) error {
	defer r.logger.Info("done")

	// Process incoming connections.
	go r.processClients()

	// Wait for all tasks to complete.
	if err := r.pruner(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}

		return err
	}

	return nil
}

// pruner waits for a prune condition to be triggered then runs a prune.
func (r *reaper) pruner(ctx context.Context) error {
	var errs []error
	resources, err := r.pruneWait(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("prune wait: %w", err))
	}

	if err = r.prune(resources); err != nil { //nolint:contextcheck // Prune needs its own context to ensure clean up completes.
		errs = append(errs, fmt.Errorf("prune: %w", err))
	}

	return errors.Join(errs...)
}

// processClients listens for incoming connections and processes them.
func (r *reaper) processClients() {
	r.logger.Info("client processing started")
	defer r.logger.Info("client processing stopped")

	for {
		conn, err := r.listener.Accept()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
				return
			}

			r.logger.Error("accept", fieldError, err)
			continue
		}

		// Block waiting for the connection to be registered
		// so that we prevent the race on connection count.
		addr := conn.RemoteAddr().String()
		select {
		case r.connected <- addr:
		case <-r.shutdown:
			// We received a new connection after shutdown started.
			// Closing without returning the ACK should trigger the caller
			// to retry and get a new reaper.
			r.logger.Warn("shutdown, aborting client", fieldAddress, addr)
			conn.Close()
			return
		}

		go r.handle(conn)
	}
}

// handle processes a connection, reading session details from
// the client and adding them to our filter.
func (r *reaper) handle(conn net.Conn) {
	addr := conn.RemoteAddr().String()
	defer func() {
		conn.Close()
		r.disconnected <- addr
	}()

	logger := r.logger.With(fieldAddress, addr)

	// Read filters from the client and add them to our list.
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg := scanner.Text()

		switch msg {
		case "":
			logger.Warn("empty filter received")
			continue
		default:
			if err := r.addFilter(msg); err != nil {
				logger.Error("add filter", fieldError, err)
				if _, err = conn.Write(ackResponse); err != nil {
					logger.Error("ack write", fieldError, err)
				}
				continue
			}

			if _, err := conn.Write(ackResponse); err != nil {
				logger.Error("ack write", fieldError, err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Error("scan", fieldError, err)
	}
}

// resources represents the resources to prune.
type resources struct {
	containers []string
	networks   []string
	volumes    []string
	images     []string
}

// shutdownListener ensures that the listener is shutdown and no new clients
// are accepted.
func (r *reaper) shutdownListener() {
	select {
	case <-r.shutdown:
		return // Already shutdown.
	default:
		close(r.shutdown)
		r.listener.Close()
	}
}

// pruneWait waits for a prune condition to be met and returns the resources to prune.
// It will retry if changes are detected.
func (r *reaper) pruneWait(ctx context.Context) (*resources, error) {
	defer r.shutdownListener()

	clients := 0
	pruneCheck := time.NewTicker(r.cfg.ConnectionTimeout)
	done := ctx.Done()
	var shutdownDeadline time.Time
	for {
		select {
		case addr := <-r.connected:
			clients++
			r.logger.Info("client connected", fieldAddress, addr, fieldClients, clients)
			if clients == 1 {
				pruneCheck.Stop()
			}
		case addr := <-r.disconnected:
			clients--
			r.logger.Info("client disconnected", fieldAddress, addr, fieldClients, clients)
			if clients == 0 {
				// No clients connected, trigger prune check overriding
				// any timeout set by shutdown signal.
				pruneCheck.Reset(r.cfg.ReconnectionTimeout)
			}
		case <-done:
			r.logger.Info("signal received", fieldClients, clients, "shutdown_timeout", r.cfg.ShutdownTimeout)
			// Force shutdown by closing the listener, scheduling
			// a pruneCheck after a timeout and setting done
			// to nil so we don't enter this case again.
			r.shutdownListener()
			shutdownDeadline = time.Now().Add(r.cfg.ShutdownTimeout)
			timeout := r.cfg.ShutdownTimeout
			if clients == 0 {
				// No clients connected, shutdown immediately.
				timeout = time.Nanosecond
			}

			pruneCheck.Reset(timeout)
			done = nil
		case now := <-pruneCheck.C:
			level := slog.LevelInfo
			if clients > 0 {
				level = slog.LevelWarn
			}
			r.logger.Log(context.Background(), level, "prune check", fieldClients, clients) //nolint:contextcheck // Ensure log is written.

			resources, err := r.resources(now.Add(r.cfg.RetryOffset)) //nolint:contextcheck // Needs its own context to ensure clean up completes.
			if err != nil {
				if errors.Is(err, errChangesDetected) {
					if shutdownDeadline.IsZero() || now.Before(shutdownDeadline) {
						r.logger.Warn("change detected, waiting again", fieldError, err)
						pruneCheck.Reset(r.cfg.ChangesRetryInterval)
						continue
					}

					// Still changes detected after shutdown timeout, force best effort prune.
					r.logger.Warn("shutdown timeout reached, forcing prune", fieldError, err)
				}

				return resources, fmt.Errorf("resources: %w", err)
			}

			return resources, nil
		}
	}
}

// resources returns the resources that match the collected filters
// for which there are no changes detected.
func (r *reaper) resources(since time.Time) (*resources, error) {
	var ret resources
	var errs []error
	// We combine errors so we can do best effort removal.
	for _, args := range r.filterArgs() {
		containers, err := r.affectedContainers(since, args)
		if err != nil {
			if !errors.Is(err, errChangesDetected) {
				r.logger.Error("affected containers", fieldError, err)
			}
			errs = append(errs, fmt.Errorf("affected containers: %w", err))
		}

		ret.containers = append(ret.containers, containers...)

		networks, err := r.affectedNetworks(since, args)
		if err != nil {
			if !errors.Is(err, errChangesDetected) {
				r.logger.Error("affected networks", fieldError, err)
			}
			errs = append(errs, fmt.Errorf("affected networks: %w", err))
		}

		ret.networks = append(ret.networks, networks...)

		volumes, err := r.affectedVolumes(since, args)
		if err != nil {
			if !errors.Is(err, errChangesDetected) {
				r.logger.Error("affected volumes", fieldError, err)
			}
			errs = append(errs, fmt.Errorf("affected volumes: %w", err))
		}

		ret.volumes = append(ret.volumes, volumes...)

		images, err := r.affectedImages(since, args)
		if err != nil {
			if !errors.Is(err, errChangesDetected) {
				r.logger.Error("affected images", fieldError, err)
			}
			errs = append(errs, fmt.Errorf("affected images: %w", err))
		}

		ret.images = append(ret.images, images...)
	}

	return &ret, errors.Join(errs...)
}

// affectedContainers returns a slice of container IDs that match the filters.
// If a matching container was created after since, an error is returned and
// the container is not included in the list.
func (r *reaper) affectedContainers(since time.Time, args filters.Args) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.RequestTimeout)
	defer cancel()

	// List all containers including stopped ones.
	options := container.ListOptions{All: true, Filters: args}
	r.logger.Debug("listing containers", "filter", options)
	containers, err := r.client.ContainerList(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("container list: %w", err)
	}

	var errChanges []error
	containerIDs := make([]string, 0, len(containers))
	for _, container := range containers {
		if container.Labels[ryukLabel] == "true" {
			// Ignore reaper containers.
			r.logger.Debug("skipping reaper container", "id", container.ID)
			continue
		}

		created := time.Unix(container.Created, 0)
		changed := created.After(since)

		r.logger.Debug("found container",
			"id", container.ID,
			"image", container.Image,
			"names", container.Names,
			"ports", container.Ports,
			"state", container.State,
			"labels", container.Labels,
			"created", created,
			"changed", changed,
			"since", since,
		)

		if changed {
			// Its not safe to remove a container which was created after
			// the prune was initiated, as this may lead to unexpected behaviour.
			errChanges = append(errChanges, fmt.Errorf("container %s: %w", container.ID, errChangesDetected))
			continue
		}

		containerIDs = append(containerIDs, container.ID)
	}

	return containerIDs, errors.Join(errChanges...)
}

// affectedNetworks returns a list of network IDs that match the filters.
// If a matching network was created after since, an error is returned and
// the network is not included in the list.
func (r *reaper) affectedNetworks(since time.Time, args filters.Args) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.RequestTimeout)
	defer cancel()

	options := network.ListOptions{Filters: args}
	r.logger.Debug("listing networks", "options", options)
	report, err := r.client.NetworkList(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("network list: %w", err)
	}

	var errChanges []error
	networks := make([]string, 0, len(report))
	for _, network := range report {
		changed := network.Created.After(since)
		r.logger.Debug("found network",
			"id", network.ID,
			"created", network.Created,
			"changed", changed,
			"since", since,
		)

		if changed {
			// Its not safe to remove a network which was created after
			// the prune was initiated, as this may lead to unexpected behaviour.
			errChanges = append(errChanges, fmt.Errorf("network %s: %w", network.ID, errChangesDetected))
			continue
		}

		networks = append(networks, network.ID)
	}

	return networks, errors.Join(errChanges...)
}

// affectedVolumes returns a list of volume names that match the filters.
// If a matching volume was created after since, an error is returned and
// the volume is not included in the list.
func (r *reaper) affectedVolumes(since time.Time, args filters.Args) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.RequestTimeout)
	defer cancel()

	options := volume.ListOptions{Filters: args}
	r.logger.Debug("listing volumes", "filter", options)
	report, err := r.client.VolumeList(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("volume list: %w", err)
	}

	var errChanges []error
	volumes := make([]string, 0, len(report.Volumes))
	for _, volume := range report.Volumes {
		created, perr := time.Parse(time.RFC3339, volume.CreatedAt)
		if perr != nil {
			// Best effort, log and continue.
			r.logger.Error("parse volume created", fieldError, perr, "volume", volume.Name)
			continue
		}

		changed := created.After(since)
		r.logger.Debug("found volume",
			"name", volume.Name,
			"created", created,
			"changed", changed,
			"since", since,
		)

		if changed {
			// Its not safe to remove a volume which was created after
			// the prune was initiated, as this may lead to unexpected behaviour.
			errChanges = append(errChanges, fmt.Errorf("volume %s: %w", volume.Name, errChangesDetected))
			continue
		}

		volumes = append(volumes, volume.Name)
	}

	return volumes, errors.Join(errChanges...)
}

// affectedImages returns a list of image IDs that match the filters.
// If a matching image was created after since, an error is returned and
// the image is not included in the list.
func (r *reaper) affectedImages(since time.Time, args filters.Args) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.RequestTimeout)
	defer cancel()

	options := image.ListOptions{Filters: args}
	r.logger.Debug("listing images", "filter", options)
	report, err := r.client.ImageList(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("image list: %w", err)
	}

	var errChanges []error
	images := make([]string, 0, len(report))
	for _, image := range report {
		created := time.Unix(image.Created, 0)
		changed := created.After(since)
		r.logger.Debug("found image",
			"id", image.ID,
			"created", created,
			"changed", changed,
			"since", since,
		)

		if changed {
			// Its not safe to remove an image which was created after
			// the prune was initiated, as this may lead to unexpected behaviour.
			errChanges = append(errChanges, fmt.Errorf("image %s: %w", image.ID, errChangesDetected))
			continue
		}

		images = append(images, image.ID)
	}

	return images, errors.Join(errChanges...)
}

// addFilter adds a filter to prune.
// Safe to call concurrently.
func (r *reaper) addFilter(msg string) error {
	query, err := url.ParseQuery(msg)
	if err != nil {
		return fmt.Errorf("parse query: %w", err)
	}

	args := filters.NewArgs()
	for filterType, values := range query {
		r.logger.Info("adding filter", "type", filterType, "values", values)
		for _, value := range values {
			args.Add(filterType, value)
		}
	}

	// We can't use msg as it could be in any order.
	data, err := args.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	key := string(data)

	r.mtx.Lock()
	defer r.mtx.Unlock()

	if _, ok := r.filters[key]; ok {
		r.logger.Debug("filter already exists", "key", key)
		return nil
	}

	r.logger.Debug("adding filter", "args", args, "key", key)
	r.filters[key] = args

	return nil
}

// filterArgs returns a slice of filter.Args to check against.
// Safe to call concurrently.
func (r *reaper) filterArgs() []filters.Args {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	filters := make([]filters.Args, 0, len(r.filters))
	for _, args := range r.filters {
		filters = append(filters, args)
	}

	return filters
}

// prune removes the specified resources.
func (r *reaper) prune(resources *resources) error {
	var containers, networks, volumes, images int
	var errs []error

	// Containers must be removed first.
	errs = append(errs, r.remove("container", resources.containers, &containers, func(ctx context.Context, id string) error {
		return r.client.ContainerRemove(ctx, id, containerRemoveOptions)
	}))

	// Networks.
	errs = append(errs, r.remove("network", resources.networks, &networks, func(ctx context.Context, id string) error {
		return r.client.NetworkRemove(ctx, id)
	}))

	// Volumes.
	errs = append(errs, r.remove("volume", resources.volumes, &volumes, func(ctx context.Context, id string) error {
		return r.client.VolumeRemove(ctx, id, volumeRemoveForce)
	}))

	// Images.
	errs = append(errs, r.remove("image", resources.images, &images, func(ctx context.Context, id string) error {
		_, err := r.client.ImageRemove(ctx, id, imageRemoveOptions)
		return err //nolint:wrapcheck // Wrapped by action.
	}))

	r.logger.Info("removed", "containers", containers, "networks", networks, "volumes", volumes, "images", images)

	return errors.Join(errs...)
}

// remove calls fn for each resource in resources and retries if necessary.
// Count is incremented for each resource that is successfully removed.
func (r *reaper) remove(resourceType string, resources []string, count *int, fn func(ctx context.Context, id string) error) error {
	logger := r.logger.With("resource", resourceType)
	logger.Debug("removing", "count", len(resources))

	if len(resources) == 0 {
		return nil
	}

	todo := make(map[string]struct{}, len(resources))
	for _, id := range resources {
		todo[id] = struct{}{}
	}

	for attempt := 1; attempt <= r.cfg.RemoveRetries; attempt++ {
		var retry bool
		for id := range todo {
			itemLogger := logger.With("id", id, "attempt", attempt)

			ctx, cancel := context.WithTimeout(context.Background(), r.cfg.RequestTimeout)
			defer cancel()

			itemLogger.Debug("remove")
			if err := fn(ctx, id); err != nil {
				if errdefs.IsNotFound(err) {
					// Already removed.
					itemLogger.Debug("not found")
					continue
				}

				itemLogger.Error("remove", fieldError, err)
				retry = true
				continue
			}

			delete(todo, id)
			*count++
		}

		if retry {
			if attempt < r.cfg.RemoveRetries {
				time.Sleep(time.Second)
			}
			continue
		}

		// All items were removed.
		return nil
	}

	// Some items were not removed.
	return fmt.Errorf("%s left %d items", resourceType, len(todo))
}
