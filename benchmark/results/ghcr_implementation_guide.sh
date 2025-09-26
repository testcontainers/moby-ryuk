#!/bin/bash

# GHCR Registry Benchmarking Implementation Guide
# Use this script template for real GHCR testing

set -e

# Configuration
GHCR_BASE="ghcr.io/testcontainers"
REPO_NAME="moby-ryuk-benchmark"
TIMESTAMP=$(date +%s)

# Prerequisites check
check_prerequisites() {
    echo "Checking prerequisites..."
    
    # Check Docker
    if ! docker info >/dev/null 2>&1; then
        echo "Error: Docker is not running"
        exit 1
    fi
    
    # Check authentication
    if [ -z "$GITHUB_TOKEN" ]; then
        echo "Error: GITHUB_TOKEN environment variable required"
        echo "Set with: export GITHUB_TOKEN=your_token"
        exit 1
    fi
    
    # Check required tools
    for tool in bc jq; do
        if ! command -v $tool >/dev/null 2>&1; then
            echo "Error: $tool is required but not installed"
            exit 1
        fi
    done
    
    echo "Prerequisites check passed"
}

# Authenticate with GHCR
authenticate_ghcr() {
    echo "Authenticating with GHCR..."
    echo "$GITHUB_TOKEN" | docker login ghcr.io -u "$GITHUB_ACTOR" --password-stdin
}

# Main execution
main() {
    check_prerequisites
    authenticate_ghcr
    
    echo "Ready for GHCR benchmarking!"
    echo "Use the registry-benchmark.sh script to run the full test suite."
}

# Run if executed directly
if [ "${BASH_SOURCE[0]}" == "${0}" ]; then
    main "$@"
fi
