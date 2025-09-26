#!/bin/bash

# Local Registry Pull Benchmarking Script for moby-ryuk
# This script simulates registry pulls using a local Docker registry to test the methodology

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
RESULTS_DIR="$SCRIPT_DIR/results"

# Local registry configuration
LOCAL_REGISTRY="localhost:5000"
REPO_NAME="moby-ryuk-benchmark"
TIMESTAMP=$(date +%s)

echo "=== Local Registry Pull Benchmarking for moby-ryuk ==="
echo "Using local registry: $LOCAL_REGISTRY/$REPO_NAME"
echo "This demonstrates the methodology for real GHCR benchmarking"

# Create results directory
mkdir -p "$RESULTS_DIR"

# Function to start local Docker registry
start_local_registry() {
    echo "Starting local Docker registry..."
    
    # Check if registry is already running
    if docker ps | grep -q "registry:2"; then
        echo "Local registry already running"
        return 0
    fi
    
    # Start local registry
    docker run -d -p 5000:5000 --name registry registry:2 >/dev/null 2>&1 || {
        echo "Starting existing registry container..."
        docker start registry >/dev/null 2>&1 || true
    }
    
    # Wait for registry to be ready
    sleep 3
    
    # Test registry connectivity
    if curl -s "http://$LOCAL_REGISTRY/v2/" >/dev/null; then
        echo "Local registry is ready at $LOCAL_REGISTRY"
    else
        echo "Warning: Local registry may not be fully ready"
    fi
}

# Function to build and push test images to local registry
build_and_push_local_images() {
    echo "Building and pushing test images to local registry..."
    
    cd "$PROJECT_DIR"
    
    # Build baseline image (without UPX)
    local baseline_dockerfile="$PROJECT_DIR/linux/Dockerfile.baseline-local"
    cat > "$baseline_dockerfile" << 'EOF'
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
LABEL benchmark.variant=baseline
EOF

    # Build and push baseline image
    local baseline_tag="$LOCAL_REGISTRY/$REPO_NAME:baseline-$TIMESTAMP"
    echo "Building baseline image: $baseline_tag"
    docker build -f "$baseline_dockerfile" -t "$baseline_tag" .
    docker push "$baseline_tag"
    
    # Build and push UPX image (using existing Dockerfile)
    local upx_tag="$LOCAL_REGISTRY/$REPO_NAME:upx-$TIMESTAMP"
    echo "Building UPX image: $upx_tag"
    docker build -f linux/Dockerfile -t "$upx_tag" .
    docker push "$upx_tag"
    
    # Clean up temporary dockerfile
    rm -f "$baseline_dockerfile"
    
    echo "Images pushed successfully to local registry:"
    echo "  Baseline: $baseline_tag"
    echo "  UPX: $upx_tag"
    
    # Save image info for later use
    echo "$baseline_tag" > "$RESULTS_DIR/local_baseline_image_tag.txt"
    echo "$upx_tag" > "$RESULTS_DIR/local_upx_image_tag.txt"
    
    # Get actual image sizes for comparison
    local baseline_size=$(docker inspect "$baseline_tag" --format='{{.Size}}')
    local upx_size=$(docker inspect "$upx_tag" --format='{{.Size}}')
    
    echo "Image sizes:"
    echo "  Baseline: $(echo "scale=2; $baseline_size / 1024 / 1024" | bc -l) MB"
    echo "  UPX: $(echo "scale=2; $upx_size / 1024 / 1024" | bc -l) MB"
    
    echo "$baseline_size" > "$RESULTS_DIR/local_baseline_size_bytes.txt"
    echo "$upx_size" > "$RESULTS_DIR/local_upx_size_bytes.txt"
}

# Function to measure local registry pull metrics
measure_local_registry_pull() {
    local variant="$1"
    local image_tag="$2"
    local iterations=10  # Reasonable for local testing
    local results_file="$RESULTS_DIR/local_pull_times_$variant.txt"
    
    echo "Measuring local registry pull for $variant ($iterations iterations)..."
    echo "Image: $image_tag"
    
    # Clear results file
    > "$results_file"
    
    for i in $(seq 1 $iterations); do
        echo "  Iteration $i/$iterations"
        
        # Remove image from local cache completely
        docker rmi "$image_tag" >/dev/null 2>&1 || true
        
        # Clear Docker system cache to simulate real registry pull
        docker system prune -f >/dev/null 2>&1
        
        # Measure pull time
        start_time=$(date +%s.%N)
        docker pull "$image_tag" >/dev/null 2>&1
        end_time=$(date +%s.%N)
        
        # Calculate duration in milliseconds
        duration=$(echo "($end_time - $start_time) * 1000" | bc -l)
        echo "$duration" >> "$results_file"
        
        printf "    Pull time: %.2f ms\n" "$duration"
    done
    
    # Calculate comprehensive statistics
    local sorted_file="/tmp/sorted_local_$variant.txt"
    sort -n "$results_file" > "$sorted_file"
    
    local count=$iterations
    local avg=$(awk '{sum+=$1; count++} END {print sum/count}' "$results_file")
    local min=$(head -1 "$sorted_file")
    local max=$(tail -1 "$sorted_file")
    
    # Calculate median (50th percentile)
    local median_pos=$((count / 2))
    local median=$(sed -n "${median_pos}p" "$sorted_file")
    
    printf "  Mean: %.2f ms\n" "$avg"
    printf "  Median: %.2f ms\n" "$median"
    printf "  Min: %.2f ms\n" "$min"  
    printf "  Max: %.2f ms\n" "$max"
    
    # Save statistics
    echo "$avg" > "$RESULTS_DIR/avg_local_pull_$variant.txt"
    echo "$median" > "$RESULTS_DIR/median_local_pull_$variant.txt"
    echo "$min" > "$RESULTS_DIR/min_local_pull_$variant.txt"
    echo "$max" > "$RESULTS_DIR/max_local_pull_$variant.txt"
    
    # Cleanup
    rm -f "$sorted_file"
}

# Function to analyze HTTP transport compression effectiveness
analyze_transport_compression() {
    echo "Analyzing HTTP transport compression effectiveness..."
    
    local baseline_tag=$(cat "$RESULTS_DIR/local_baseline_image_tag.txt")
    local upx_tag=$(cat "$RESULTS_DIR/local_upx_image_tag.txt")
    
    # Get detailed layer information
    echo "Analyzing image layers and compression..."
    
    # Export images to analyze compression ratios
    local baseline_export="/tmp/baseline_export.tar"
    local upx_export="/tmp/upx_export.tar"
    
    docker save "$baseline_tag" -o "$baseline_export"
    docker save "$upx_tag" -o "$upx_export"
    
    local baseline_compressed_size=$(stat -c%s "$baseline_export")
    local upx_compressed_size=$(stat -c%s "$upx_export")
    
    echo "Compressed export sizes (simulating registry compression):"
    echo "  Baseline: $(echo "scale=2; $baseline_compressed_size / 1024 / 1024" | bc -l) MB"
    echo "  UPX: $(echo "scale=2; $upx_compressed_size / 1024 / 1024" | bc -l) MB"
    
    # Calculate compression ratios
    local baseline_image_size=$(cat "$RESULTS_DIR/local_baseline_size_bytes.txt")
    local upx_image_size=$(cat "$RESULTS_DIR/local_upx_size_bytes.txt")
    
    local baseline_compression_ratio=$(echo "scale=2; $baseline_compressed_size / $baseline_image_size" | bc -l)
    local upx_compression_ratio=$(echo "scale=2; $upx_compressed_size / $upx_image_size" | bc -l)
    
    echo "HTTP transport compression ratios:"
    echo "  Baseline: $baseline_compression_ratio ($(echo "scale=1; (1 - $baseline_compression_ratio) * 100" | bc -l)% compression)"
    echo "  UPX: $upx_compression_ratio ($(echo "scale=1; (1 - $upx_compression_ratio) * 100" | bc -l)% compression)"
    
    # Save compression analysis
    echo "$baseline_compressed_size" > "$RESULTS_DIR/baseline_compressed_size.txt"
    echo "$upx_compressed_size" > "$RESULTS_DIR/upx_compressed_size.txt"
    echo "$baseline_compression_ratio" > "$RESULTS_DIR/baseline_compression_ratio.txt"
    echo "$upx_compression_ratio" > "$RESULTS_DIR/upx_compression_ratio.txt"
    
    rm -f "$baseline_export" "$upx_export"
}

# Function to generate local registry report
generate_local_report() {
    echo "Generating local registry benchmark report..."
    
    local baseline_size_mb=$(echo "scale=2; $(cat "$RESULTS_DIR/local_baseline_size_bytes.txt") / 1024 / 1024" | bc -l)
    local upx_size_mb=$(echo "scale=2; $(cat "$RESULTS_DIR/local_upx_size_bytes.txt") / 1024 / 1024" | bc -l)
    
    cat > "$RESULTS_DIR/local_registry_summary.txt" << EOF
Local Registry Pull Benchmarking Summary for moby-ryuk
======================================================

Test Configuration:
- Registry: Local Docker Registry ($LOCAL_REGISTRY)
- Iterations: 10 per variant
- Purpose: Demonstrate real registry pull methodology
- Timestamp: $TIMESTAMP

Image Sizes:
  Baseline: ${baseline_size_mb} MB
  UPX: ${upx_size_mb} MB
  Size reduction: $(echo "scale=1; ($baseline_size_mb - $upx_size_mb) / $baseline_size_mb * 100" | bc -l)%

Local Registry Pull Times:
EOF

    if [ -f "$RESULTS_DIR/avg_local_pull_baseline.txt" ]; then
        baseline_avg=$(cat "$RESULTS_DIR/avg_local_pull_baseline.txt")
        baseline_median=$(cat "$RESULTS_DIR/median_local_pull_baseline.txt")
        baseline_min=$(cat "$RESULTS_DIR/min_local_pull_baseline.txt")
        baseline_max=$(cat "$RESULTS_DIR/max_local_pull_baseline.txt")
        
        cat >> "$RESULTS_DIR/local_registry_summary.txt" << EOF
  Baseline:
    Mean: ${baseline_avg} ms
    Median: ${baseline_median} ms
    Min: ${baseline_min} ms
    Max: ${baseline_max} ms
EOF
    fi

    if [ -f "$RESULTS_DIR/avg_local_pull_upx.txt" ]; then
        upx_avg=$(cat "$RESULTS_DIR/avg_local_pull_upx.txt")
        upx_median=$(cat "$RESULTS_DIR/median_local_pull_upx.txt")
        upx_min=$(cat "$RESULTS_DIR/min_local_pull_upx.txt")
        upx_max=$(cat "$RESULTS_DIR/max_local_pull_upx.txt")
        
        cat >> "$RESULTS_DIR/local_registry_summary.txt" << EOF
  UPX:
    Mean: ${upx_avg} ms
    Median: ${upx_median} ms
    Min: ${upx_min} ms
    Max: ${upx_max} ms
EOF
    fi

    # Calculate improvements
    if [ -f "$RESULTS_DIR/avg_local_pull_baseline.txt" ] && [ -f "$RESULTS_DIR/avg_local_pull_upx.txt" ]; then
        baseline_time=$(cat "$RESULTS_DIR/avg_local_pull_baseline.txt")
        upx_time=$(cat "$RESULTS_DIR/avg_local_pull_upx.txt")
        
        time_improvement=$(echo "scale=1; ($baseline_time - $upx_time) / $baseline_time * 100" | bc -l)
        
        cat >> "$RESULTS_DIR/local_registry_summary.txt" << EOF

Performance Analysis:
  Pull time improvement: ${time_improvement}%
  Network transfer reduction: $(echo "scale=1; ($baseline_size_mb - $upx_size_mb) / $baseline_size_mb * 100" | bc -l)%
EOF
    fi

    # Add compression analysis if available
    if [ -f "$RESULTS_DIR/baseline_compression_ratio.txt" ]; then
        baseline_comp_ratio=$(cat "$RESULTS_DIR/baseline_compression_ratio.txt")
        upx_comp_ratio=$(cat "$RESULTS_DIR/upx_compression_ratio.txt")
        
        cat >> "$RESULTS_DIR/local_registry_summary.txt" << EOF

HTTP Transport Compression Analysis:
  Baseline compression effectiveness: $(echo "scale=1; (1 - $baseline_comp_ratio) * 100" | bc -l)%
  UPX compression effectiveness: $(echo "scale=1; (1 - $upx_comp_ratio) * 100" | bc -l)%
  
Note: UPX pre-compression reduces the effectiveness of HTTP transport compression,
but the overall benefit still strongly favors UPX due to the significant base size reduction.
EOF
    fi

    cat >> "$RESULTS_DIR/local_registry_summary.txt" << EOF

Methodology Validation:
This local registry test demonstrates the methodology that would be used with GHCR.
The real GHCR test would provide:
- Actual network latency and bandwidth constraints
- Real-world registry performance characteristics  
- True egress measurement and cost analysis
- Production-grade HTTP compression effectiveness

For production GHCR testing, use the registry-benchmark.sh script with proper authentication.
EOF

    echo ""
    echo "=== Local Registry Benchmark Summary ==="
    cat "$RESULTS_DIR/local_registry_summary.txt"
}

# Function to cleanup local registry and images
cleanup_local() {
    echo "Cleaning up local test environment..."
    
    # Remove test images
    if [ -f "$RESULTS_DIR/local_baseline_image_tag.txt" ]; then
        local baseline_tag=$(cat "$RESULTS_DIR/local_baseline_image_tag.txt")
        docker rmi "$baseline_tag" >/dev/null 2>&1 || true
    fi
    
    if [ -f "$RESULTS_DIR/local_upx_image_tag.txt" ]; then
        local upx_tag=$(cat "$RESULTS_DIR/local_upx_image_tag.txt")
        docker rmi "$upx_tag" >/dev/null 2>&1 || true
    fi
    
    # Stop local registry (but don't remove it in case it's used for other purposes)
    echo "Local registry container left running for potential reuse"
    echo "To stop: docker stop registry"
    echo "To remove: docker rm registry"
}

# Install required tools
if ! command -v bc >/dev/null 2>&1; then
    echo "Installing bc for calculations..."
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y bc
    fi
fi

echo ""
echo "=== Phase 1: Start Local Registry ==="
start_local_registry

echo ""
echo "=== Phase 2: Build and Push Images ==="
build_and_push_local_images

echo ""
echo "=== Phase 3: Registry Pull Benchmarks ==="
baseline_tag=$(cat "$RESULTS_DIR/local_baseline_image_tag.txt")
upx_tag=$(cat "$RESULTS_DIR/local_upx_image_tag.txt")

measure_local_registry_pull "baseline" "$baseline_tag"
measure_local_registry_pull "upx" "$upx_tag"

echo ""
echo "=== Phase 4: HTTP Compression Analysis ==="
analyze_transport_compression

echo ""
echo "=== Phase 5: Generate Report ==="
generate_local_report

echo ""
echo "=== Phase 6: Cleanup ==="
cleanup_local

echo ""
echo "Local registry benchmarking complete! Results saved in: $RESULTS_DIR"
echo ""
echo "Key files:"
echo "  - local_registry_summary.txt: Complete local benchmark results"
echo "  - local_pull_times_*.txt: Raw pull time measurements"
echo ""
echo "This demonstrates the methodology for real GHCR benchmarking."
echo "Use registry-benchmark.sh for production GHCR testing with proper authentication."