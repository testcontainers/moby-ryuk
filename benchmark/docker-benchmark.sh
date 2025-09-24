#!/bin/bash

# Docker Image Benchmarking Script for moby-ryuk
# This script measures the impact of UPX compression on Docker image sizes and pull times

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
RESULTS_DIR="$SCRIPT_DIR/results"

echo "=== Docker Image UPX Benchmarking for moby-ryuk ==="

# Create results directory
mkdir -p "$RESULTS_DIR"

# Function to build Docker image and measure size
build_and_measure_docker() {
    local variant="$1"
    local dockerfile="$2"
    local tag="testcontainers/ryuk:benchmark-$variant"
    
    echo "Building Docker image: $tag"
    
    # Build the image
    cd "$PROJECT_DIR"
    docker build -f "$dockerfile" -t "$tag" .
    
    # Get image size
    local size_info=$(docker images "$tag" --format "table {{.Size}}" | tail -n 1)
    local size_bytes=$(docker inspect "$tag" --format='{{.Size}}')
    local size_mb=$(echo "scale=2; $size_bytes / 1024 / 1024" | bc -l)
    
    echo "Image $variant size: $size_info (${size_mb} MB)"
    
    # Save results
    echo "$size_bytes" > "$RESULTS_DIR/docker_size_bytes_$variant.txt"
    echo "$size_mb" > "$RESULTS_DIR/docker_size_mb_$variant.txt"
    
    return 0
}

# Function to measure image pull time (simulated by save/load)
measure_pull_time() {
    local variant="$1"
    local tag="testcontainers/ryuk:benchmark-$variant"
    local iterations=5
    local results_file="$RESULTS_DIR/pull_times_$variant.txt"
    
    echo "Measuring pull time simulation for $variant ($iterations iterations)..."
    
    # Clear results file
    > "$results_file"
    
    # Export image to simulate registry pull
    local temp_file="/tmp/ryuk-$variant.tar"
    docker save "$tag" -o "$temp_file"
    
    for i in $(seq 1 $iterations); do
        # Remove image from local cache
        docker rmi "$tag" >/dev/null 2>&1 || true
        
        # Measure time to load (simulates pull)
        start_time=$(date +%s.%N)
        docker load -i "$temp_file" >/dev/null 2>&1
        end_time=$(date +%s.%N)
        
        # Calculate duration in milliseconds
        duration=$(echo "($end_time - $start_time) * 1000" | bc -l)
        echo "$duration" >> "$results_file"
        printf "  Iteration %d: %.2f ms\n" "$i" "$duration"
    done
    
    # Cleanup temp file
    rm -f "$temp_file"
    
    # Calculate statistics
    local avg=$(awk '{sum+=$1; count++} END {print sum/count}' "$results_file")
    local min=$(sort -n "$results_file" | head -1)
    local max=$(sort -n "$results_file" | tail -1)
    
    printf "  Average: %.2f ms\n" "$avg"
    printf "  Min: %.2f ms\n" "$min"  
    printf "  Max: %.2f ms\n" "$max"
    
    echo "$avg" > "$RESULTS_DIR/avg_pull_$variant.txt"
}

# Function to create a baseline Dockerfile (without UPX)
create_baseline_dockerfile() {
    local dockerfile="$PROJECT_DIR/linux/Dockerfile.baseline"
    
    cat > "$dockerfile" << 'EOF'
# -----------
# Build Image
# -----------
FROM golang:1.23-alpine3.22 AS build

WORKDIR /app

# Go build env
ENV CGO_ENABLED=0

# Install source deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source & build
COPY --link . .

# Build binary (baseline - original approach)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags '-s' -o /bin/ryuk

# -----------------
# Certificates
# -----------------
FROM alpine:3.22 AS certs

RUN apk --no-cache add ca-certificates

# -----------------
# Distributed Image
# -----------------
FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /bin/ryuk /bin/ryuk
CMD ["/bin/ryuk"]
LABEL org.testcontainers.ryuk=true
EOF
}

# Function to create an optimized Dockerfile (without UPX)
create_optimized_dockerfile() {
    local dockerfile="$PROJECT_DIR/linux/Dockerfile.optimized"
    
    cat > "$dockerfile" << 'EOF'
# -----------
# Build Image
# -----------
FROM golang:1.23-alpine3.22 AS build

WORKDIR /app

# Go build env
ENV CGO_ENABLED=0

# Install source deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source & build
COPY --link . .

# Build binary (optimized but no UPX)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build \
    -a \
    -installsuffix cgo \
    -ldflags="-w -s" \
    -trimpath \
    -o /bin/ryuk .

# -----------------
# Certificates
# -----------------
FROM alpine:3.22 AS certs

RUN apk --no-cache add ca-certificates

# -----------------
# Distributed Image
# -----------------
FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /bin/ryuk /bin/ryuk
CMD ["/bin/ryuk"]
LABEL org.testcontainers.ryuk=true
EOF
}

# Install bc for calculations if not available
if ! command -v bc >/dev/null 2>&1; then
    echo "Installing bc for calculations..."
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y bc
    fi
fi

echo ""
echo "=== Creating Dockerfiles ==="

create_baseline_dockerfile
create_optimized_dockerfile

echo ""
echo "=== Building Docker Images ==="

# Build baseline image (original approach)
build_and_measure_docker "baseline" "linux/Dockerfile.baseline"

# Build optimized image (no UPX)
build_and_measure_docker "optimized" "linux/Dockerfile.optimized"

# Build UPX-compressed image (current modified Dockerfile)
build_and_measure_docker "upx" "linux/Dockerfile"

echo ""
echo "=== Measuring Pull Times ==="

measure_pull_time "baseline"
measure_pull_time "optimized"
measure_pull_time "upx"

echo ""
echo "=== Calculating Docker Results ==="

# Generate Docker summary report
cat > "$RESULTS_DIR/docker_summary.txt" << EOF
Docker Image UPX Benchmarking Summary for moby-ryuk
===================================================

Image Sizes:
EOF

if [ -f "$RESULTS_DIR/docker_size_mb_baseline.txt" ]; then
    baseline_size=$(cat "$RESULTS_DIR/docker_size_mb_baseline.txt")
    echo "  Baseline Image: ${baseline_size} MB" >> "$RESULTS_DIR/docker_summary.txt"
fi

if [ -f "$RESULTS_DIR/docker_size_mb_optimized.txt" ]; then
    optimized_size=$(cat "$RESULTS_DIR/docker_size_mb_optimized.txt")
    echo "  Optimized Image: ${optimized_size} MB" >> "$RESULTS_DIR/docker_summary.txt"
fi

if [ -f "$RESULTS_DIR/docker_size_mb_upx.txt" ]; then
    upx_size=$(cat "$RESULTS_DIR/docker_size_mb_upx.txt")
    echo "  UPX Image: ${upx_size} MB" >> "$RESULTS_DIR/docker_summary.txt"
fi

echo "" >> "$RESULTS_DIR/docker_summary.txt"
echo "Pull Times (Average):" >> "$RESULTS_DIR/docker_summary.txt"

if [ -f "$RESULTS_DIR/avg_pull_baseline.txt" ]; then
    baseline_time=$(cat "$RESULTS_DIR/avg_pull_baseline.txt")
    echo "  Baseline: ${baseline_time} ms" >> "$RESULTS_DIR/docker_summary.txt"
fi

if [ -f "$RESULTS_DIR/avg_pull_optimized.txt" ]; then
    optimized_time=$(cat "$RESULTS_DIR/avg_pull_optimized.txt")
    echo "  Optimized: ${optimized_time} ms" >> "$RESULTS_DIR/docker_summary.txt"
fi

if [ -f "$RESULTS_DIR/avg_pull_upx.txt" ]; then
    upx_time=$(cat "$RESULTS_DIR/avg_pull_upx.txt")
    echo "  UPX: ${upx_time} ms" >> "$RESULTS_DIR/docker_summary.txt"
fi

# Calculate image size reduction percentages
if [ -f "$RESULTS_DIR/docker_size_mb_baseline.txt" ] && [ -f "$RESULTS_DIR/docker_size_mb_upx.txt" ]; then
    baseline_size=$(cat "$RESULTS_DIR/docker_size_mb_baseline.txt")
    upx_size=$(cat "$RESULTS_DIR/docker_size_mb_upx.txt")
    reduction=$(echo "scale=1; ($baseline_size - $upx_size) / $baseline_size * 100" | bc -l)
    echo "" >> "$RESULTS_DIR/docker_summary.txt"
    echo "Image Size Reduction: ${reduction}%" >> "$RESULTS_DIR/docker_summary.txt"
fi

# Calculate pull time difference
if [ -f "$RESULTS_DIR/avg_pull_baseline.txt" ] && [ -f "$RESULTS_DIR/avg_pull_upx.txt" ]; then
    baseline_time=$(cat "$RESULTS_DIR/avg_pull_baseline.txt")
    upx_time=$(cat "$RESULTS_DIR/avg_pull_upx.txt")
    improvement=$(echo "scale=1; ($baseline_time - $upx_time) / $baseline_time * 100" | bc -l)
    echo "Pull Time Improvement: ${improvement}%" >> "$RESULTS_DIR/docker_summary.txt"
fi

echo ""
echo "=== Docker Summary ==="
cat "$RESULTS_DIR/docker_summary.txt"

echo ""
echo "=== Cleanup ==="
docker rmi testcontainers/ryuk:benchmark-baseline >/dev/null 2>&1 || true
docker rmi testcontainers/ryuk:benchmark-optimized >/dev/null 2>&1 || true  
docker rmi testcontainers/ryuk:benchmark-upx >/dev/null 2>&1 || true
rm -f "$PROJECT_DIR/linux/Dockerfile.baseline" "$PROJECT_DIR/linux/Dockerfile.optimized"

echo ""
echo "Docker benchmarking complete! Results saved in: $RESULTS_DIR"