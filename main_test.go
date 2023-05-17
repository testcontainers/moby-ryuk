package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

var addr = &net.TCPAddr{
	IP:   net.IPv4zero,
	Port: 5555,
	Zone: "",
}

var testConnectionTimeout time.Duration = 5 * time.Second

func init() {
	reconnectionTimeout = 1 * time.Second
}

func TestReconnectionTimeout(t *testing.T) {
	// reset connectionTimeout
	connectionTimeout = testConnectionTimeout

	acc := make(chan net.Addr)
	lost := make(chan net.Addr)

	done := make(chan struct{})

	go func() {
		waitForPruneCondition(context.Background(), acc, lost)
		done <- struct{}{}
	}()

	acc <- addr
	lost <- addr

	select {
	case <-done:
		return
	case <-time.After(2 * time.Second):
		t.Fail()
	}
}

func TestInitialTimeout(t *testing.T) {
	// reset connectionTimeout
	connectionTimeout = testConnectionTimeout

	acc := make(chan net.Addr)
	lost := make(chan net.Addr)

	done := make(chan string)

	go func() {
		defer func() {
			err := recover().(string)
			done <- err
		}()
		waitForPruneCondition(context.Background(), acc, lost)
	}()

	select {
	case p := <-done:
		if !strings.Contains(p, "first connection") {
			t.Fail()
		}
	case <-time.After(7 * time.Second):
		t.Fail()
	}
}

func TestPrune(t *testing.T) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	cli.NegotiateAPIVersion(context.Background())

	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err = cli.Ping(ctx)
	}

	if err != nil {
		t.Fatal(err)
	}
	cli.NegotiateAPIVersion(context.Background())

	maxLength := 25

	t.Run("Empty death note", func(t *testing.T) {
		deathNote := &sync.Map{}

		dc, dn, dv, di := prune(cli, deathNote)
		assert.Equal(t, 0, dc)
		assert.Equal(t, 0, dn)
		assert.Equal(t, 0, dv)
		assert.Equal(t, 0, di)
	})

	t.Run("Malformed death note", func(t *testing.T) {
		deathNote := &sync.Map{}
		deathNote.Store("param", true)

		dc, dn, dv, di := prune(cli, deathNote)
		assert.Equal(t, 0, dc)
		assert.Equal(t, 0, dn)
		assert.Equal(t, 0, dv)
		assert.Equal(t, 0, di)
	})

	t.Run("Malformed JSON death note", func(t *testing.T) {
		deathNote := &sync.Map{}
		deathNote.Store(`{"label": "color"}`, true)

		dc, dn, dv, di := prune(cli, deathNote)
		assert.Equal(t, 0, dc)
		assert.Equal(t, 0, dn)
		assert.Equal(t, 0, dv)
		assert.Equal(t, 0, di)
	})

	t.Run("Death note removing containers", func(t *testing.T) {
		const label = "removable-container"
		deathNote := &sync.Map{}
		deathNote.Store(`{"label": {"`+label+`=true": true}}`, true)

		ctx := context.Background()
		for i := 0; i < maxLength; i++ {
			c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
				ContainerRequest: testcontainers.ContainerRequest{
					Image: "nginx:alpine",
					Labels: map[string]string{
						label: "true",
					},
					SkipReaper: true,
				},
				Started: true,
			})
			require.Nil(t, err)
			require.NotNil(t, c)
		}

		dc, dn, dv, di := prune(cli, deathNote)
		assert.Equal(t, maxLength, dc)
		assert.Equal(t, 0, dn)
		assert.Equal(t, 0, dv)
		assert.Equal(t, 0, di)
	})

	t.Run("Death note skips reaper container itself", func(t *testing.T) {
		const label = "removable-container"
		deathNote := &sync.Map{}
		deathNote.Store(`{"label": {"`+label+`=true": true}}`, true)

		ctx := context.Background()
		c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "nginx:alpine",
				Labels: map[string]string{
					label:                     "true",
					"org.testcontainers.ryuk": "true",
				},
				SkipReaper: true,
			},
			Started: true,
		})
		require.Nil(t, err)
		require.NotNil(t, c)

		dc, _, _, _ := prune(cli, deathNote)
		assert.Equal(t, 0, dc)

		err = c.Terminate(ctx)
		require.Nil(t, err)
	})

	t.Run("Death note removing networks", func(t *testing.T) {
		const label = "removable-network"
		deathNote := &sync.Map{}
		deathNote.Store(`{"label": {"`+label+`=true": true}}`, true)

		ctx := context.Background()
		for i := 0; i < maxLength; i++ {
			network, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
				NetworkRequest: testcontainers.NetworkRequest{
					Labels: map[string]string{
						label: "true",
					},
					Name: fmt.Sprintf("ryuk-network-%d", i),
				},
			})
			require.Nil(t, err)
			require.NotNil(t, network)
			t.Cleanup(func() {
				_ = network.Remove(ctx)
			})
		}

		dc, dn, dv, di := prune(cli, deathNote)
		assert.Equal(t, 0, dc)
		assert.Equal(t, maxLength, dn)
		assert.Equal(t, 0, dv)
		assert.Equal(t, 0, di)
	})

	t.Run("Death note removing volumes", func(t *testing.T) {
		const label = "removable-volume"
		deathNote := &sync.Map{}
		deathNote.Store(`{"label": {"`+label+`=true": true}}`, true)

		ctx := context.Background()
		for i := 0; i < maxLength; i++ {
			opts := volume.CreateOptions{
				Name: fmt.Sprintf("volume-%d", i),
				Labels: map[string]string{
					label: "true",
				},
			}

			vol, err := cli.VolumeCreate(ctx, opts)
			require.Nil(t, err)
			require.NotNil(t, vol)
			t.Cleanup(func() {
				_ = cli.VolumeRemove(ctx, vol.Name, true)
			})
		}

		dc, dn, dv, di := prune(cli, deathNote)
		assert.Equal(t, 0, dc)
		assert.Equal(t, 0, dn)
		assert.Equal(t, maxLength, dv)
		assert.Equal(t, 0, di)
	})

	t.Run("Death note removing images", func(t *testing.T) {
		const label = "removable-image"
		deathNote := &sync.Map{}
		deathNote.Store(`{"label": {"`+label+`=true": true}}`, true)

		ctx := context.Background()
		for i := 0; i < maxLength; i++ {
			buf := new(bytes.Buffer)
			tw := tar.NewWriter(buf)
			defer tw.Close()

			dockerFile := "Dockerfile"
			dockerFileReader, err := os.Open(filepath.Join("testresources", dockerFile))
			require.Nil(t, err)

			readDockerFile, err := io.ReadAll(dockerFileReader)
			require.Nil(t, err)

			tarHeader := &tar.Header{
				Name: dockerFile,
				Size: int64(len(readDockerFile)),
			}
			err = tw.WriteHeader(tarHeader)
			require.Nil(t, err)

			_, err = tw.Write(readDockerFile)
			require.Nil(t, err)
			dockerFileTarReader := bytes.NewReader(buf.Bytes())

			opt := types.ImageBuildOptions{
				Remove:      true,
				ForceRemove: true, // removing containers produced by the build
				Labels: map[string]string{
					label:   "true",
					"index": fmt.Sprintf("%d", i),
				},
				Context:    dockerFileTarReader,
				Dockerfile: dockerFile,
				Tags:       []string{fmt.Sprintf("moby-ryuk:test-%d", i)}, // adding a tag so that image is not marked as 'dangling'
			}

			response, err := cli.ImageBuild(ctx, dockerFileTarReader, opt)
			require.Nil(t, err)
			require.NotNil(t, response)

			// need to read the response from Docker before continuing the execution
			buf = new(bytes.Buffer)
			_, err = buf.ReadFrom(response.Body)
			require.Nil(t, err)

			err = response.Body.Close()
			require.Nil(t, err)
		}

		dc, dn, dv, di := prune(cli, deathNote)

		assert.Equal(t, 0, dc)
		assert.Equal(t, 0, dn)
		assert.Equal(t, 0, dv)
		assert.Equal(t, maxLength, di)
	})
}

func Test_newConfig(t *testing.T) {
	t.Run("should return an error when failing to parse RYUK_CONNECTION_TIMEOUT environment variable", func(t *testing.T) {
		t.Setenv(connectionTimeoutEnv, "bad_value")

		config, err := newConfig([]string{})
		require.NotNil(t, err)
		require.Nil(t, config)
	})

	t.Run("should set connectionTimeout with RYUK_CONNECTION_TIMEOUT environment variable", func(t *testing.T) {
		t.Setenv(connectionTimeoutEnv, "10s")

		config, err := newConfig([]string{})
		require.Nil(t, err)
		assert.Equal(t, 10*time.Second, config.ConnectionTimeout)
	})

	t.Run("should return an error when failing to parse RYUK_PORT environment variable", func(t *testing.T) {
		t.Setenv(portEnv, "bad_value")

		config, err := newConfig([]string{})
		require.NotNil(t, err)
		require.Nil(t, config)
	})

	t.Run("should set connectionTimeout with RYUK_PORT environment variable", func(t *testing.T) {
		t.Setenv(portEnv, "8081")

		config, err := newConfig([]string{})
		require.Nil(t, err)
		assert.Equal(t, 8081, config.Port)
	})

	t.Run("should return an error when failing to parse RYUK_RECONNECTION_TIMEOUT environment variable", func(t *testing.T) {
		t.Setenv(reconnectionTimeoutEnv, "bad_value")

		config, err := newConfig([]string{})
		require.NotNil(t, err)
		require.Nil(t, config)
	})

	t.Run("should set connectionTimeout with RYUK_RECONNECTION_TIMEOUT environment variable", func(t *testing.T) {
		t.Setenv(reconnectionTimeoutEnv, "100s")

		config, err := newConfig([]string{})
		require.Nil(t, err)
		assert.Equal(t, 100*time.Second, config.ReconnectionTimeout)
	})

	t.Run("should return an error when failing to parse RYUK_VERBOSE environment variable", func(t *testing.T) {
		t.Setenv(verboseEnv, "bad_value")

		config, err := newConfig([]string{})
		require.NotNil(t, err)
		require.Nil(t, config)
	})

	t.Run("should set verbose with RYUK_VERBOSE environment variable", func(t *testing.T) {
		t.Setenv(verboseEnv, "true")

		config, err := newConfig([]string{})
		require.Nil(t, err)
		assert.True(t, config.Verbose)

		t.Setenv(verboseEnv, "false")

		config, err = newConfig([]string{})
		require.Nil(t, err)
		assert.False(t, config.Verbose)
	})

	t.Run("should set port with port flag", func(t *testing.T) {
		config, err := newConfig([]string{"-p", "3000"})
		require.Nil(t, err)
		assert.Equal(t, 3000, config.Port)
	})

	t.Run("should set port from env with port flag and RYUK_PORT environment variable", func(t *testing.T) {
		t.Setenv(portEnv, "8081")

		config, err := newConfig([]string{"-p", "3000"})
		require.Nil(t, err)
		assert.Equal(t, 8081, config.Port)
	})
}
