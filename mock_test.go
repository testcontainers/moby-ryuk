package main

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/stretchr/testify/mock"
)

var _ dockerClient = (*mockClient)(nil)

type mockClient struct {
	mock.Mock
}

func (c *mockClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	args := c.Called(ctx, options)
	return args.Get(0).([]container.Summary), args.Error(1)
}

func (c *mockClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	args := c.Called(ctx, containerID, options)
	return args.Error(0)
}

func (c *mockClient) ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
	args := c.Called(ctx, options)
	return args.Get(0).([]image.Summary), args.Error(1)
}

func (c *mockClient) ImageRemove(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
	args := c.Called(ctx, imageID, options)
	return args.Get(0).([]image.DeleteResponse), args.Error(1)
}

func (c *mockClient) NetworkList(ctx context.Context, options network.ListOptions) ([]network.Summary, error) {
	args := c.Called(ctx, options)
	return args.Get(0).([]network.Summary), args.Error(1)
}

func (c *mockClient) NetworkRemove(ctx context.Context, networkID string) error {
	args := c.Called(ctx, networkID)
	return args.Error(0)
}

func (c *mockClient) VolumeList(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error) {
	args := c.Called(ctx, options)
	return args.Get(0).(volume.ListResponse), args.Error(1)
}

func (c *mockClient) VolumeRemove(ctx context.Context, volumeID string, force bool) error {
	args := c.Called(ctx, volumeID, force)
	return args.Error(0)
}

func (c *mockClient) Ping(ctx context.Context) (types.Ping, error) {
	args := c.Called(ctx)
	return args.Get(0).(types.Ping), args.Error(1)
}

func (c *mockClient) NegotiateAPIVersion(ctx context.Context) {
	c.Called(ctx)
}
