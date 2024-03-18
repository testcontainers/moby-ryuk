# Releasing Moby Ryuk

With Moby Ryuk primary use case in the Testcontainers framework, it is normally used as container and not as plain binary.
So releases are published as Docker image to https://hub.docker.com/r/testcontainers/ryuk.

The `.github/workflows/publish-docker-image.yml` workflow is our primary release workflow,
while preview images are published via `.github/workflows/build-docker-image.yml`.

## Multi-Architecture support

The workflows publishing the `testcontainers/ryuk` Docker image ensure that multiple platforms are supported.
Supported platforms are listed in the "supported-architectures.txt" file. The file is used as reference for
the checks after publishing the multi-arch image.
