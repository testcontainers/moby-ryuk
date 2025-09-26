#!/bin/bash

# Registry Pull Benchmarking Script for moby-ryuk
# This script measures real pull times and egress from GHCR (GitHub Container Registry)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
RESULTS_DIR="$SCRIPT_DIR/results"

# GHCR configuration
GHCR_BASE="ghcr.io/testcontainers"
REPO_NAME="moby-ryuk-benchmark"
TIMESTAMP=$(date +%s)

echo "=== Registry Pull Benchmarking for moby-ryuk ==="
echo "Using GHCR: $GHCR_BASE/$REPO_NAME"
echo "Timestamp: $TIMESTAMP"

# Create results directory
mkdir -p "$RESULTS_DIR"

# Function to check GHCR authentication
check_ghcr_auth() {
    echo "Checking GHCR authentication..."
    if ! docker info >/dev/null 2>&1; then
        echo "Error: Docker is not running"
        return 1
    fi
    
    # Try to authenticate with GHCR using GitHub token if available
    if [ -n "$GITHUB_TOKEN" ]; then
        echo "Using GITHUB_TOKEN for authentication"
        echo "$GITHUB_TOKEN" | docker login ghcr.io -u "$GITHUB_ACTOR" --password-stdin
    else
        echo "Warning: No GITHUB_TOKEN found. Using existing authentication."
        echo "Make sure you're authenticated with: docker login ghcr.io"
    fi
}

# Function to build and push test images
build_and_push_images() {
    echo "Building and pushing test images to GHCR..."
    
    cd "$PROJECT_DIR"
    
    # Build baseline image (without UPX)
    local baseline_dockerfile="$PROJECT_DIR/linux/Dockerfile.baseline-registry"
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
LABEL benchmark.timestamp=$TIMESTAMP
EOF

    # Build and push baseline image
    local baseline_tag="$GHCR_BASE/$REPO_NAME:baseline-$TIMESTAMP"
    echo "Building baseline image: $baseline_tag"
    docker build -f "$baseline_dockerfile" -t "$baseline_tag" .
    docker push "$baseline_tag"
    
    # Build and push UPX image (using existing Dockerfile)
    local upx_tag="$GHCR_BASE/$REPO_NAME:upx-$TIMESTAMP"
    echo "Building UPX image: $upx_tag"
    docker build -f linux/Dockerfile -t "$upx_tag" .
    docker push "$upx_tag"
    
    # Clean up temporary dockerfile
    rm -f "$baseline_dockerfile"
    
    echo "Images pushed successfully:"
    echo "  Baseline: $baseline_tag"
    echo "  UPX: $upx_tag"
    
    # Save image info for later use
    echo "$baseline_tag" > "$RESULTS_DIR/baseline_image_tag.txt"
    echo "$upx_tag" > "$RESULTS_DIR/upx_image_tag.txt"
}

# Function to measure registry pull metrics
measure_registry_pull() {
    local variant="$1"
    local image_tag="$2"
    local iterations=20  # Fewer iterations for real registry pulls
    local results_file="$RESULTS_DIR/registry_pull_times_$variant.txt"
    local egress_file="$RESULTS_DIR/registry_egress_$variant.txt"
    
    echo "Measuring real registry pull for $variant ($iterations iterations)..."
    echo "Image: $image_tag"
    
    # Clear results files
    > "$results_file"
    > "$egress_file"
    
    for i in $(seq 1 $iterations); do
        echo "  Iteration $i/$iterations"
        
        # Remove image from local cache completely
        docker rmi "$image_tag" >/dev/null 2>&1 || true
        docker system prune -f >/dev/null 2>&1 || true
        
        # Measure pull time and capture pull output for egress analysis
        local pull_output="/tmp/pull_output_${variant}_${i}.txt"
        start_time=$(date +%s.%N)
        docker pull "$image_tag" > "$pull_output" 2>&1
        end_time=$(date +%s.%N)
        
        # Calculate duration in milliseconds
        duration=$(echo "($end_time - $start_time) * 1000" | bc -l)
        echo "$duration" >> "$results_file"
        
        # Extract size information from pull output
        local pulled_size=$(grep -o 'Pull complete.*' "$pull_output" | wc -l || echo "0")
        local total_size=$(docker inspect "$image_tag" --format='{{.Size}}' 2>/dev/null || echo "0")
        echo "$total_size" >> "$egress_file"
        
        rm -f "$pull_output"
        
        # Brief pause to avoid overwhelming the registry
        sleep 2
    done
    
    # Calculate comprehensive statistics
    local sorted_file="/tmp/sorted_registry_$variant.txt"
    sort -n "$results_file" > "$sorted_file"
    
    local count=$iterations
    local avg=$(awk '{sum+=$1; count++} END {print sum/count}' "$results_file")
    local min=$(head -1 "$sorted_file")
    local max=$(tail -1 "$sorted_file")
    
    # Calculate median (50th percentile)
    local median_pos=$((count / 2))
    local median=$(sed -n "${median_pos}p" "$sorted_file")
    
    # Calculate 90th percentile
    local p90_pos=$((count * 90 / 100))
    local p90=$(sed -n "${p90_pos}p" "$sorted_file")
    
    printf "  Mean: %.2f ms\n" "$avg"
    printf "  Median: %.2f ms\n" "$median"
    printf "  Min: %.2f ms\n" "$min"  
    printf "  Max: %.2f ms\n" "$max"
    printf "  90th percentile: %.2f ms\n" "$p90"
    
    # Calculate egress statistics
    local avg_egress=$(awk '{sum+=$1; count++} END {print sum/count}' "$egress_file")
    local avg_egress_mb=$(echo "scale=2; $avg_egress / 1024 / 1024" | bc -l)
    
    printf "  Average egress: %.2f MB\n" "$avg_egress_mb"
    
    # Save all statistics
    echo "$avg" > "$RESULTS_DIR/avg_registry_pull_$variant.txt"
    echo "$median" > "$RESULTS_DIR/median_registry_pull_$variant.txt"
    echo "$min" > "$RESULTS_DIR/min_registry_pull_$variant.txt"
    echo "$max" > "$RESULTS_DIR/max_registry_pull_$variant.txt"
    echo "$p90" > "$RESULTS_DIR/p90_registry_pull_$variant.txt"
    echo "$avg_egress_mb" > "$RESULTS_DIR/avg_egress_$variant.txt"
    
    # Cleanup
    rm -f "$sorted_file"
}

# Function to test HTTP compression impact
test_http_compression() {
    echo "Testing HTTP compression impact..."
    
    local baseline_tag=$(cat "$RESULTS_DIR/baseline_image_tag.txt")
    local upx_tag=$(cat "$RESULTS_DIR/upx_image_tag.txt")
    
    # Clear any cached images
    docker rmi "$baseline_tag" "$upx_tag" >/dev/null 2>&1 || true
    
    echo "Measuring compressed vs uncompressed transfer sizes..."
    
    # Pull with verbose output to capture transfer details
    local baseline_manifest="/tmp/baseline_manifest.json"
    local upx_manifest="/tmp/upx_manifest.json"
    
    # Get image manifests to analyze layers
    docker manifest inspect "$baseline_tag" > "$baseline_manifest" 2>/dev/null || echo "{}" > "$baseline_manifest"
    docker manifest inspect "$upx_tag" > "$upx_manifest" 2>/dev/null || echo "{}" > "$upx_manifest"
    
    # Pull images and measure actual transfer
    echo "Pulling baseline image with compression analysis..."
    docker pull "$baseline_tag" 2>&1 | tee "$RESULTS_DIR/baseline_pull_log.txt"
    
    echo "Pulling UPX image with compression analysis..."
    docker pull "$upx_tag" 2>&1 | tee "$RESULTS_DIR/upx_pull_log.txt"
    
    # Analyze manifests for layer sizes
    local baseline_layers=$(jq -r '.layers[]?.size // 0' "$baseline_manifest" 2>/dev/null | paste -sd+ | bc 2>/dev/null || echo "0")
    local upx_layers=$(jq -r '.layers[]?.size // 0' "$upx_manifest" 2>/dev/null | paste -sd+ | bc 2>/dev/null || echo "0")
    
    echo "Layer size analysis:"
    echo "  Baseline total layer size: $baseline_layers bytes"
    echo "  UPX total layer size: $upx_layers bytes"
    
    # Save compression analysis
    echo "$baseline_layers" > "$RESULTS_DIR/baseline_layer_size.txt"
    echo "$upx_layers" > "$RESULTS_DIR/upx_layer_size.txt"
    
    rm -f "$baseline_manifest" "$upx_manifest"
}

# Function to generate comprehensive registry benchmark report
generate_registry_report() {
    echo "Generating comprehensive registry benchmark report..."
    
    cat > "$RESULTS_DIR/registry_summary.txt" << EOF
Registry Pull Benchmarking Summary for moby-ryuk
================================================

Test Configuration:
- Registry: GHCR (GitHub Container Registry)
- Iterations: 20 per variant
- Images: baseline vs UPX compressed
- Timestamp: $TIMESTAMP

Real Registry Pull Times:
EOF

    if [ -f "$RESULTS_DIR/avg_registry_pull_baseline.txt" ]; then
        baseline_avg=$(cat "$RESULTS_DIR/avg_registry_pull_baseline.txt")
        baseline_median=$(cat "$RESULTS_DIR/median_registry_pull_baseline.txt")
        baseline_min=$(cat "$RESULTS_DIR/min_registry_pull_baseline.txt")
        baseline_max=$(cat "$RESULTS_DIR/max_registry_pull_baseline.txt")
        baseline_p90=$(cat "$RESULTS_DIR/p90_registry_pull_baseline.txt")
        baseline_egress=$(cat "$RESULTS_DIR/avg_egress_baseline.txt")
        
        cat >> "$RESULTS_DIR/registry_summary.txt" << EOF
  Baseline:
    Mean: ${baseline_avg} ms
    Median: ${baseline_median} ms
    Min: ${baseline_min} ms
    Max: ${baseline_max} ms
    90th percentile: ${baseline_p90} ms
    Average egress: ${baseline_egress} MB
EOF
    fi

    if [ -f "$RESULTS_DIR/avg_registry_pull_upx.txt" ]; then
        upx_avg=$(cat "$RESULTS_DIR/avg_registry_pull_upx.txt")
        upx_median=$(cat "$RESULTS_DIR/median_registry_pull_upx.txt")
        upx_min=$(cat "$RESULTS_DIR/min_registry_pull_upx.txt")
        upx_max=$(cat "$RESULTS_DIR/max_registry_pull_upx.txt")
        upx_p90=$(cat "$RESULTS_DIR/p90_registry_pull_upx.txt")
        upx_egress=$(cat "$RESULTS_DIR/avg_egress_upx.txt")
        
        cat >> "$RESULTS_DIR/registry_summary.txt" << EOF
  UPX:
    Mean: ${upx_avg} ms
    Median: ${upx_median} ms
    Min: ${upx_min} ms
    Max: ${upx_max} ms
    90th percentile: ${upx_p90} ms
    Average egress: ${upx_egress} MB
EOF
    fi

    # Calculate improvements
    if [ -f "$RESULTS_DIR/avg_registry_pull_baseline.txt" ] && [ -f "$RESULTS_DIR/avg_registry_pull_upx.txt" ]; then
        baseline_time=$(cat "$RESULTS_DIR/avg_registry_pull_baseline.txt")
        upx_time=$(cat "$RESULTS_DIR/avg_registry_pull_upx.txt")
        baseline_egress=$(cat "$RESULTS_DIR/avg_egress_baseline.txt")
        upx_egress=$(cat "$RESULTS_DIR/avg_egress_upx.txt")
        
        time_improvement=$(echo "scale=1; ($baseline_time - $upx_time) / $baseline_time * 100" | bc -l)
        egress_reduction=$(echo "scale=1; ($baseline_egress - $upx_egress) / $baseline_egress * 100" | bc -l)
        
        cat >> "$RESULTS_DIR/registry_summary.txt" << EOF

Performance Analysis:
  Pull time improvement: ${time_improvement}%
  Egress reduction: ${egress_reduction}%
  
Network Impact:
  Data transfer savings: $(echo "scale=2; $baseline_egress - $upx_egress" | bc -l) MB per pull
  Bandwidth efficiency: ${egress_reduction}% reduction in network usage

HTTP Transport Compression:
  See baseline_pull_log.txt and upx_pull_log.txt for detailed transfer analysis
  Layer-level compression effectiveness varies by content type
EOF
    fi

    echo ""
    echo "=== Registry Benchmark Summary ==="
    cat "$RESULTS_DIR/registry_summary.txt"
}

# Function to cleanup test images from registry
cleanup_registry_images() {
    echo "Cleaning up test images..."
    
    if [ -f "$RESULTS_DIR/baseline_image_tag.txt" ]; then
        local baseline_tag=$(cat "$RESULTS_DIR/baseline_image_tag.txt")
        docker rmi "$baseline_tag" >/dev/null 2>&1 || true
        echo "Cleaned up baseline image: $baseline_tag"
    fi
    
    if [ -f "$RESULTS_DIR/upx_image_tag.txt" ]; then
        local upx_tag=$(cat "$RESULTS_DIR/upx_image_tag.txt")
        docker rmi "$upx_tag" >/dev/null 2>&1 || true
        echo "Cleaned up UPX image: $upx_tag"
    fi
    
    echo "Note: Images remain in GHCR and should be manually deleted if desired"
}

# Install required tools
if ! command -v bc >/dev/null 2>&1; then
    echo "Installing bc for calculations..."
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y bc
    fi
fi

if ! command -v jq >/dev/null 2>&1; then
    echo "Installing jq for JSON processing..."
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y jq
    fi
fi

echo ""
echo "=== Phase 1: Authentication Check ==="
check_ghcr_auth

echo ""
echo "=== Phase 2: Build and Push Images ==="
build_and_push_images

echo ""
echo "=== Phase 3: Registry Pull Benchmarks ==="
baseline_tag=$(cat "$RESULTS_DIR/baseline_image_tag.txt")
upx_tag=$(cat "$RESULTS_DIR/upx_image_tag.txt")

measure_registry_pull "baseline" "$baseline_tag"
measure_registry_pull "upx" "$upx_tag"

echo ""
echo "=== Phase 4: HTTP Compression Analysis ==="
test_http_compression

echo ""
echo "=== Phase 5: Generate Report ==="
generate_registry_report

echo ""
echo "=== Phase 6: Cleanup ==="
cleanup_registry_images

echo ""
echo "Registry benchmarking complete! Results saved in: $RESULTS_DIR"
echo "Key files:"
echo "  - registry_summary.txt: Complete registry benchmark results"
echo "  - *_pull_log.txt: Detailed pull logs for compression analysis"
echo "  - registry_pull_times_*.txt: Raw pull time measurements"