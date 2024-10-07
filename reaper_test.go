package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	containerID1     = "container1"
	containerID2     = "container2"
	networkID1       = "network1"
	networkID2       = "network2"
	volumeName1      = "volume1"
	volumeName2      = "volume2"
	imageID1         = "image1"
	imageID2         = "image2"
	testImage        = "alpine:latest"
	imageBuildResult = "moby.image.id"
)

var (
	// testConfig is a config used for testing.
	testConfig = withConfig(config{
		Port:                0,
		ConnectionTimeout:   time.Millisecond * 500,
		ReconnectionTimeout: time.Millisecond * 100,
		RequestTimeout:      time.Millisecond * 50,
		ShutdownTimeout:     time.Second * 2,
		RemoveRetries:       1,
		RetryOffset:         -time.Second * 2,
		Verbose:             true,
	})

	// discardLogger is a logger that discards all logs.
	discardLogger = withLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// testLabels is a set of test labels.
	testLabels = map[string]string{
		labelBase:                "true",
		labelBase + ".sessionID": "test-session",
		labelBase + ".version":   "0.1.0",
	}

	// mockContext is a matcher that matches any context.
	mockContext = mock.MatchedBy(func(context.Context) bool { return true })

	// errNotFound is a docker not found error.
	errNotFound = errdefs.NotFound(errors.New("not found"))
)

func Test_newReaper(t *testing.T) {
	ctx := context.Background()
	t.Run("basic", func(t *testing.T) {
		r, err := newReaper(ctx, discardLogger, testConfig)
		require.NoError(t, err)
		require.NotNil(t, r)
	})

	t.Run("with-config", func(t *testing.T) {
		r, err := newReaper(ctx, discardLogger, testConfig)
		require.NoError(t, err)
		require.NotNil(t, r)
	})

	t.Run("bad-config", func(t *testing.T) {
		r, err := newReaper(ctx, discardLogger, withConfig(config{}))
		require.Error(t, err)
		require.Nil(t, r)
	})

	t.Run("with-client", func(t *testing.T) {
		cli := &mockClient{}
		cli.On("Ping", mockContext).Return(types.Ping{}, nil)
		cli.On("NegotiateAPIVersion", mockContext).Return()
		r, err := newReaper(ctx, discardLogger, testConfig, withClient(cli))
		require.NoError(t, err)
		require.NotNil(t, r)
	})
}

// testConnect connects to the given endpoint, sends filter labels,
// and expects an ACK. The connection is closed when the context is done.
func testConnect(ctx context.Context, t *testing.T, endpoint string) {
	t.Helper()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", endpoint)
	require.NoError(t, err)

	labelFilters := make([]string, 0, len(testLabels))
	for l, v := range testLabels {
		labelFilters = append(labelFilters, fmt.Sprintf("label=%s=%s", l, v))
	}

	_, err = conn.Write([]byte(strings.Join(labelFilters, "&") + "\n"))
	require.NoError(t, err)

	buf := make([]byte, 4)
	n, err := conn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "ACK\n", string(buf[:n]))

	go func() {
		defer conn.Close()
		<-ctx.Done()
	}()
}

// runTest is a test case for the reaper run method.
type runTest struct {
	createdAt1 time.Time
	pingErr    error

	containerListErr    error
	containerRemoveErr1 error
	containerRemoveErr2 error
	containerCreated2   time.Time

	networkListErr    error
	networkRemoveErr1 error
	networkRemoveErr2 error
	networkCreated2   time.Time

	volumeListErr    error
	volumeRemoveErr1 error
	volumeRemoveErr2 error
	volumeCreated2   time.Time

	imageListErr    error
	imageRemoveErr1 error
	imageRemoveErr2 error
	imageCreated2   time.Time
}

// newRunTest returns a new runTest with created at times set in the past.
func newRunTest() *runTest {
	now := time.Now().Add(-time.Minute)
	return &runTest{
		createdAt1:        now,
		containerCreated2: now,
		networkCreated2:   now,
		volumeCreated2:    now,
		imageCreated2:     now,
	}
}

// newMockClient returns a new mock client for the given test case.
func newMockClient(tc *runTest) *mockClient {
	cli := &mockClient{}
	cli.On("Ping", mock.Anything).Return(types.Ping{}, tc.pingErr)
	cli.On("NegotiateAPIVersion", mockContext).Return()

	// Mock the container list and remove calls.
	cli.On("ContainerList", mockContext, mock.Anything).Return([]types.Container{
		{
			ID:      containerID1,
			Created: tc.createdAt1.Unix(),
			Image:   "testcontainers/test1:latest",
			Names:   []string{"test1"},
			Ports: []types.Port{{
				PrivatePort: 1001,
				PublicPort:  8081,
				Type:        "tcp",
			}},
			State:  "running",
			Labels: testLabels,
		},
		{
			ID:      containerID2,
			Created: tc.containerCreated2.Unix(),
			Image:   "testcontainers/test2:latest",
			Names:   []string{"test2"},
			Ports: []types.Port{{
				PrivatePort: 1002,
				PublicPort:  8082,
				Type:        "tcp",
			}},
			State:  "running",
			Labels: testLabels,
		},
	}, tc.containerListErr)

	cli.On("ContainerRemove", mockContext, containerID1, containerRemoveOptions).
		Return(tc.containerRemoveErr1)
	cli.On("ContainerRemove", mockContext, containerID2, containerRemoveOptions).
		Return(tc.containerRemoveErr2)

	// Mock the network list and remove calls.
	cli.On("NetworkList", mockContext, mock.Anything).
		Return([]network.Summary{
			{ID: networkID1, Created: tc.createdAt1},
			{ID: networkID2, Created: tc.networkCreated2},
		}, tc.networkListErr)
	cli.On("NetworkRemove", mockContext, networkID1).
		Return(tc.networkRemoveErr1)
	cli.On("NetworkRemove", mockContext, networkID2).
		Return(tc.networkRemoveErr2)

	// Mock the volume list and remove calls.
	cli.On("VolumeList", mockContext, mock.Anything).
		Return(volume.ListResponse{
			Volumes: []*volume.Volume{
				{Name: volumeName1, CreatedAt: tc.createdAt1.Format(time.RFC3339)},
				{Name: volumeName2, CreatedAt: tc.volumeCreated2.Format(time.RFC3339)},
			},
		}, tc.volumeListErr)
	cli.On("VolumeRemove", mockContext, volumeName1, volumeRemoveForce).
		Return(tc.volumeRemoveErr1)
	cli.On("VolumeRemove", mockContext, volumeName2, volumeRemoveForce).
		Return(tc.volumeRemoveErr2)

	// Mock the image list and remove calls.
	cli.On("ImageList", mockContext, mock.Anything).Return([]image.Summary{
		{ID: imageID1, Created: tc.createdAt1.Unix()},
		{ID: imageID2, Created: tc.imageCreated2.Unix()},
	}, tc.imageListErr)
	cli.On("ImageRemove", mockContext, imageID1, imageRemoveOptions).
		Return([]image.DeleteResponse{{Deleted: imageID1}}, tc.imageRemoveErr1)
	cli.On("ImageRemove", mockContext, imageID2, imageRemoveOptions).
		Return([]image.DeleteResponse{{Deleted: imageID2}}, tc.imageRemoveErr2)

	return cli
}

// testReaperRun runs the reaper with the given test case and returns the log output.
func testReaperRun(t *testing.T, tc *runTest) (string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	t.Cleanup(cancel)

	var buf safeBuffer
	logger := withLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	client := newMockClient(tc)
	r, err := newReaper(ctx, logger, withClient(client), testConfig)
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- r.run(ctx)
	}()

	clientCtx, clientCancel := context.WithTimeout(ctx, time.Millisecond*500)
	t.Cleanup(clientCancel)

	addr := r.listener.Addr().String()
	testConnect(clientCtx, t, addr)
	testConnect(clientCtx, t, addr)

	select {
	case err = <-errCh:
	case <-ctx.Done():
		t.Fatal("timeout", buf.String())
	}

	// Standard checks for basic functionality.
	log := buf.String()
	require.Contains(t, log, "listening address="+addr)
	require.Contains(t, log, "client connected")
	require.Contains(t, log, "adding filter")

	return log, err
}

func Test_newReaper_Run(t *testing.T) {
	t.Run("end-to-end", func(t *testing.T) {
		tc := newRunTest()
		log, err := testReaperRun(t, tc)
		require.NoError(t, err)

		require.NotContains(t, log, "level=ERROR")
		require.NotContains(t, log, "level=WARN")
		require.Contains(t, log, "removed containers=2 networks=2 volumes=2 images=2")
	})

	t.Run("container-created", func(t *testing.T) {
		tc := newRunTest()
		tc.containerCreated2 = time.Now().Add(time.Millisecond * 200)
		log, err := testReaperRun(t, tc)
		require.NoError(t, err)

		require.NotContains(t, log, "level=ERROR")
		require.Contains(t, log, `msg="change detected, waiting again" error="affected containers: container container2: changes detected"`)
		require.Contains(t, log, "removed containers=2 networks=2 volumes=2 images=2")
	})

	t.Run("network-created", func(t *testing.T) {
		tc := newRunTest()
		tc.networkCreated2 = time.Now().Add(time.Millisecond * 200)
		log, err := testReaperRun(t, tc)
		require.NoError(t, err)

		require.NotContains(t, log, "level=ERROR")
		require.Contains(t, log, `msg="change detected, waiting again" error="affected networks: network network2: changes detected"`)
		require.Contains(t, log, "removed containers=2 networks=2 volumes=2 images=2")
	})

	t.Run("volume-created", func(t *testing.T) {
		tc := newRunTest()
		tc.volumeCreated2 = time.Now().Add(time.Millisecond * 200)
		log, err := testReaperRun(t, tc)
		require.NoError(t, err)

		require.NotContains(t, log, "level=ERROR")
		require.Contains(t, log, `msg="change detected, waiting again" error="affected volumes: volume volume2: changes detected"`)
		require.Contains(t, log, "removed containers=2 networks=2 volumes=2 images=2")
	})

	t.Run("image-created", func(t *testing.T) {
		tc := newRunTest()
		tc.imageCreated2 = time.Now().Add(time.Millisecond * 200)
		log, err := testReaperRun(t, tc)
		require.NoError(t, err)

		require.NotContains(t, log, "level=ERROR")
		require.Contains(t, log, `msg="change detected, waiting again" error="affected images: image image2: changes detected"`)
		require.Contains(t, log, "removed containers=2 networks=2 volumes=2 images=2")
	})

	t.Run("not-found", func(t *testing.T) {
		tc := newRunTest()
		tc.containerRemoveErr1 = errNotFound
		tc.networkRemoveErr1 = errNotFound
		tc.volumeRemoveErr1 = errNotFound
		tc.imageRemoveErr1 = errNotFound
		log, err := testReaperRun(t, tc)
		require.NoError(t, err)

		require.NotContains(t, log, "level=ERROR")
		require.NotContains(t, log, "level=WARN")
		require.Contains(t, log, "removed containers=1 networks=1 volumes=1 images=1")
	})

	t.Run("container-remove-error", func(t *testing.T) {
		tc := newRunTest()
		tc.containerRemoveErr1 = errors.New("remove error")
		log, err := testReaperRun(t, tc)
		require.EqualError(t, err, "prune: container left 1 items")

		require.Contains(t, log, "level=ERROR")
		require.NotContains(t, log, "level=WARN")
		require.Contains(t, log, "removed containers=1 networks=2 volumes=2 images=2")
	})

	t.Run("network-remove-error", func(t *testing.T) {
		tc := newRunTest()
		tc.networkRemoveErr1 = errors.New("remove error")
		log, err := testReaperRun(t, tc)
		require.EqualError(t, err, "prune: network left 1 items")

		require.Contains(t, log, "level=ERROR")
		require.NotContains(t, log, "level=WARN")
		require.Contains(t, log, "removed containers=2 networks=1 volumes=2 images=2")
	})

	t.Run("volume-remove-error", func(t *testing.T) {
		tc := newRunTest()
		tc.volumeRemoveErr1 = errors.New("remove error")
		log, err := testReaperRun(t, tc)
		require.EqualError(t, err, "prune: volume left 1 items")

		require.Contains(t, log, "level=ERROR")
		require.NotContains(t, log, "level=WARN")
		require.Contains(t, log, "removed containers=2 networks=2 volumes=1 images=2")
	})

	t.Run("image-remove-error", func(t *testing.T) {
		tc := newRunTest()
		tc.imageRemoveErr1 = errors.New("remove error")
		log, err := testReaperRun(t, tc)
		require.EqualError(t, err, "prune: image left 1 items")

		require.Contains(t, log, "level=ERROR")
		require.NotContains(t, log, "level=WARN")
		require.Contains(t, log, "removed containers=2 networks=2 volumes=2 images=1")
	})

	t.Run("container-list-error", func(t *testing.T) {
		tc := newRunTest()
		tc.containerListErr = errors.New("list error")
		log, err := testReaperRun(t, tc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container list: "+tc.containerListErr.Error())

		require.Contains(t, log, "level=ERROR")
		require.NotContains(t, log, "level=WARN")
		require.Contains(t, log, "removed containers=0 networks=2 volumes=2 images=2")
	})

	t.Run("network-list-error", func(t *testing.T) {
		tc := newRunTest()
		tc.networkListErr = errors.New("list error")
		log, err := testReaperRun(t, tc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "network list: "+tc.networkListErr.Error())

		require.Contains(t, log, "level=ERROR")
		require.NotContains(t, log, "level=WARN")
		require.Contains(t, log, "removed containers=2 networks=0 volumes=2 images=2")
	})

	t.Run("volume-list-error", func(t *testing.T) {
		tc := newRunTest()
		tc.volumeListErr = errors.New("list error")
		log, err := testReaperRun(t, tc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "volume list: "+tc.volumeListErr.Error())

		require.Contains(t, log, "level=ERROR")
		require.NotContains(t, log, "level=WARN")
		require.Contains(t, log, "removed containers=2 networks=2 volumes=0 images=2")
	})

	t.Run("image-list-error", func(t *testing.T) {
		tc := newRunTest()
		tc.imageListErr = errors.New("list error")
		log, err := testReaperRun(t, tc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "image list: "+tc.imageListErr.Error())

		require.Contains(t, log, "level=ERROR")
		require.NotContains(t, log, "level=WARN")
		require.Contains(t, log, "removed containers=2 networks=2 volumes=2 images=0")
	})
}

// safeBuffer is a buffer safe for concurrent use.
type safeBuffer struct {
	buf bytes.Buffer
	mtx sync.Mutex
}

// Write writes to the buffer.
func (sb *safeBuffer) Write(p []byte) (n int, err error) {
	sb.mtx.Lock()
	defer sb.mtx.Unlock()

	return sb.buf.Write(p)
}

// String returns the buffer as a string.
func (sb *safeBuffer) String() string {
	sb.mtx.Lock()
	defer sb.mtx.Unlock()

	return sb.buf.String()
}

func TestAbortedClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	t.Cleanup(cancel)

	var log safeBuffer
	logger := withLogger(slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	tc := newRunTest()
	cli := newMockClient(tc)
	r, err := newReaper(ctx, logger, withClient(cli), testConfig)
	require.NoError(t, err)

	// Start processing clients.
	go r.processClients()

	// Fake a shutdown to trigger the client abort.
	close(r.shutdown)

	addr := r.listener.Addr().String()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	require.NoError(t, err)

	// With shutdown triggered the client should be aborted.
	// The write will still succeed due to buffering but the read will fail.
	_, err = conn.Write([]byte("some-filter\n"))
	require.NoError(t, err)

	buf := make([]byte, 4)
	n, err := conn.Read(buf)
	require.Error(t, err)
	switch {
	case errors.Is(err, io.EOF),
		errors.Is(err, syscall.ECONNRESET):
		// Expected errors.
	default:
		t.Fatal("unexpected read error:", err)
	}
	require.Zero(t, n)
	require.Contains(t, log.String(), "shutdown, aborting client")
}

func TestShutdownTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	t.Cleanup(cancel)

	var log safeBuffer
	logger := withLogger(slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	tc := newRunTest()
	cli := newMockClient(tc)
	r, err := newReaper(ctx, logger, withClient(cli), testConfig)
	require.NoError(t, err)

	errCh := make(chan error, 1)
	runCtx, runCancel := context.WithCancel(ctx)
	t.Cleanup(runCancel)
	go func() {
		errCh <- r.run(runCtx)
	}()

	require.NoError(t, err)

	testConnect(ctx, t, r.listener.Addr().String())
	runCancel()

	select {
	case err = <-errCh:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("timeout", log.String())
	}

	require.Contains(t, log.String(), "shutdown timeout")
}

func TestReapContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	t.Cleanup(cancel)

	// Run a test container.
	cli := testClient(t)
	config := &container.Config{
		Image:  testImage,
		Cmd:    []string{"sleep", "10"},
		Labels: testLabels,
	}
	resp, err := cli.ContainerCreate(ctx, config, nil, nil, nil, testID())
	if errdefs.IsNotFound(err) {
		// Image not found, pull it.
		var rc io.ReadCloser
		rc, err = cli.ImagePull(ctx, testImage, image.PullOptions{})
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, rc.Close())
		})
		_, err = io.Copy(io.Discard, rc)
		require.NoError(t, err)
		resp, err = cli.ContainerCreate(ctx, config, nil, nil, nil, testID())
	}
	require.NoError(t, err)

	t.Cleanup(func() {
		// Ensure the container was / is removed.
		err := cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{})
		require.Error(t, err)
		require.True(t, errdefs.IsNotFound(err))
	})

	// Speed up reaper for testing.
	t.Setenv("RYUK_RECONNECTION_TIMEOUT", "10ms")
	t.Setenv("RYUK_SHUTDOWN_TIMEOUT", "1s")
	t.Setenv("RYUK_PORT", "0")

	testReaper(ctx, t,
		"msg=removed containers=1 networks=0 volumes=0 images=0",
		"msg=remove resource=container id="+resp.ID,
	)
}

func TestReapNetwork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	t.Cleanup(cancel)

	// Create a test network.
	cli := testClient(t)
	resp, err := cli.NetworkCreate(ctx, testID(), network.CreateOptions{
		Labels: testLabels,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		// Ensure the network was / is removed.
		err := cli.NetworkRemove(ctx, resp.ID)
		require.Error(t, err)
		require.True(t, errdefs.IsNotFound(err))
	})

	testReaper(ctx, t,
		"msg=removed containers=0 networks=1 volumes=0 images=0",
		"msg=remove resource=network id="+resp.ID,
	)
}

func TestReapVolume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	t.Cleanup(cancel)

	// Create a test volume.
	cli := testClient(t)
	resp, err := cli.VolumeCreate(ctx, volume.CreateOptions{
		Labels: testLabels,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		// Ensure the volume was / is removed.
		err := cli.VolumeRemove(ctx, resp.Name, false)
		require.Error(t, err)
		require.True(t, errdefs.IsNotFound(err))
	})

	testReaper(ctx, t,
		"msg=removed containers=0 networks=0 volumes=1 images=0",
		"msg=remove resource=volume id="+resp.Name,
	)
}

func TestReapImage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1000)
	t.Cleanup(cancel)

	// Create a test image.
	cli := testClient(t)
	context, err := archive.Tar("testdata", archive.Uncompressed)
	require.NoError(t, err)
	resp, err := cli.ImageBuild(ctx, context, types.ImageBuildOptions{
		Version: types.BuilderBuildKit,
		Labels:  testLabels,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})

	// Process the build output, discarding it so we catch any
	// errors and get the image ID.
	var imageID string
	auxCallback := func(msg jsonmessage.JSONMessage) {
		if msg.ID != imageBuildResult {
			return
		}
		var result types.BuildResult
		err := json.Unmarshal(*msg.Aux, &result)
		require.NoError(t, err)
		imageID = result.ID
	}
	err = jsonmessage.DisplayJSONMessagesStream(resp.Body, io.Discard, 0, false, auxCallback)
	require.NoError(t, err)
	require.NotEmpty(t, imageID)

	t.Cleanup(func() {
		// Ensure the image was / is removed.
		resp, err := cli.ImageRemove(ctx, imageID, image.RemoveOptions{})
		require.Error(t, err)
		require.Empty(t, resp)
	})

	testReaper(ctx, t,
		"msg=removed containers=0 networks=0 volumes=0 images=1",
		"msg=remove resource=image id="+imageID,
	)
}

// testID returns a unique test ID.
func testID() string {
	return fmt.Sprintf("test-%d", time.Now().UnixNano())
}

// testClient returns a new docker client for testing.
func testClient(t *testing.T) *client.Client {
	t.Helper()

	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, cli.Close())
	})

	return cli
}

// testReaper runs the reaper with test labels and validates that expect
// strings are found in the log output.
func testReaper(ctx context.Context, t *testing.T, expect ...string) {
	t.Helper()

	// Speed up reaper for testing and use a random port.
	t.Setenv("RYUK_RECONNECTION_TIMEOUT", "10ms")
	t.Setenv("RYUK_SHUTDOWN_TIMEOUT", "1s")
	t.Setenv("RYUK_PORT", "0")

	// Start the reaper.
	var log safeBuffer
	logger := withLogger(slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	r, err := newReaper(ctx, logger)
	require.NoError(t, err)

	reaperErr := make(chan error, 1)
	go func() {
		reaperErr <- r.run(ctx)
	}()

	// Inform the reaper of the labels to reap.
	addr := r.listener.Addr().String()
	clientCtx, clientCancel := context.WithCancel(ctx)
	t.Cleanup(clientCancel) // Ensure the clientCtx is cancelled on failure.
	testConnect(clientCtx, t, addr)
	clientCancel()

	select {
	case err = <-reaperErr:
		require.NoError(t, err)

		data := log.String()
		for _, e := range expect {
			require.Contains(t, data, e)
		}
	case <-ctx.Done():
		t.Fatal("timeout", log.String())
	}
}
