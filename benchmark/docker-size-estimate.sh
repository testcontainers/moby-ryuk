#!/bin/bash

# Docker Image Size Estimation Script for moby-ryuk
# This script estimates Docker image size differences based on binary sizes

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
RESULTS_DIR="$SCRIPT_DIR/results"

echo "=== Docker Image Size Estimation for moby-ryuk ==="

# Create results directory if it doesn't exist
mkdir -p "$RESULTS_DIR"

# Check if binary benchmark results exist
if [ ! -f "$RESULTS_DIR/size_mb_baseline.txt" ]; then
    echo "Binary benchmark results not found. Please run benchmark.sh first."
    exit 1
fi

# Read binary sizes
baseline_binary_mb=$(cat "$RESULTS_DIR/size_mb_baseline.txt")
optimized_binary_mb=$(cat "$RESULTS_DIR/size_mb_optimized.txt")
upx_binary_mb=$(cat "$RESULTS_DIR/size_mb_upx.txt")

echo "Binary sizes from benchmarks:"
echo "  Baseline: $baseline_binary_mb MB"
echo "  Optimized: $optimized_binary_mb MB"
echo "  UPX: $upx_binary_mb MB"

# Estimate base container overhead (scratch + ca-certificates)
# Based on typical scratch container with ca-certificates: ~200KB
base_overhead_mb=0.2

# Calculate estimated Docker image sizes
baseline_image_mb=$(echo "scale=2; $baseline_binary_mb + $base_overhead_mb" | bc -l)
optimized_image_mb=$(echo "scale=2; $optimized_binary_mb + $base_overhead_mb" | bc -l)
upx_image_mb=$(echo "scale=2; $upx_binary_mb + $base_overhead_mb" | bc -l)

echo ""
echo "Estimated Docker image sizes:"
echo "  Baseline image: $baseline_image_mb MB"
echo "  Optimized image: $optimized_image_mb MB"
echo "  UPX image: $upx_image_mb MB"

# Calculate reductions
binary_reduction=$(echo "scale=1; ($baseline_binary_mb - $upx_binary_mb) / $baseline_binary_mb * 100" | bc -l)
image_reduction=$(echo "scale=1; ($baseline_image_mb - $upx_image_mb) / $baseline_image_mb * 100" | bc -l)

echo ""
echo "Size reductions:"
echo "  Binary reduction: $binary_reduction%"
echo "  Image reduction: $image_reduction%"

# Save results for analysis
echo "$baseline_image_mb" > "$RESULTS_DIR/docker_size_mb_baseline.txt"
echo "$optimized_image_mb" > "$RESULTS_DIR/docker_size_mb_optimized.txt"
echo "$upx_image_mb" > "$RESULTS_DIR/docker_size_mb_upx.txt"

# Create Docker summary
cat > "$RESULTS_DIR/docker_summary.txt" << EOF
Docker Image Size Estimation Summary for moby-ryuk
==================================================

Estimated Image Sizes:
  Baseline Image: ${baseline_image_mb} MB
  Optimized Image: ${optimized_image_mb} MB
  UPX Image: ${upx_image_mb} MB

Size Reductions:
  Binary Size Reduction: ${binary_reduction}%
  Image Size Reduction: ${image_reduction}%

Notes:
- Estimates based on scratch container + ca-certificates (~0.2MB overhead)
- Actual sizes may vary slightly due to Docker layer compression
- UPX provides significant size reduction for both binary and image
EOF

echo ""
echo "=== Docker Size Estimation Summary ==="
cat "$RESULTS_DIR/docker_summary.txt"

# Simulate pull time benefits based on size reduction
# Assume 10 Mbps connection (typical CI/CD scenario)
connection_mbps=10
baseline_pull_seconds=$(echo "scale=1; $baseline_image_mb * 8 / $connection_mbps" | bc -l)
upx_pull_seconds=$(echo "scale=1; $upx_image_mb * 8 / $connection_mbps" | bc -l)
pull_time_savings=$(echo "scale=1; $baseline_pull_seconds - $upx_pull_seconds" | bc -l)

echo ""
echo "Pull Time Analysis (10 Mbps connection):"
echo "  Baseline pull time: $baseline_pull_seconds seconds"
echo "  UPX pull time: $upx_pull_seconds seconds"
echo "  Time savings: $pull_time_savings seconds"

# Save pull time estimates
echo "$baseline_pull_seconds" > "$RESULTS_DIR/estimated_pull_baseline.txt"
echo "$upx_pull_seconds" > "$RESULTS_DIR/estimated_pull_upx.txt" 
echo "$pull_time_savings" > "$RESULTS_DIR/estimated_pull_savings.txt"

echo ""
echo "Docker size estimation complete! Results saved in: $RESULTS_DIR"