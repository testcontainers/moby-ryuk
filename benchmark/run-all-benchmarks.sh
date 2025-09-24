#!/bin/bash

# Master benchmarking script for UPX impact analysis
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="$SCRIPT_DIR/results"

echo "=== Complete UPX Benchmarking Suite for moby-ryuk ==="
echo "This will run binary and Docker image benchmarks..."

# Clean up any previous results
rm -rf "$RESULTS_DIR"
mkdir -p "$RESULTS_DIR"

echo ""
echo "=== Running Binary Benchmarks ==="
"$SCRIPT_DIR/benchmark.sh"

echo ""
echo "=== Running Docker Image Benchmarks ==="  
"$SCRIPT_DIR/docker-benchmark.sh"

echo ""
echo "=== Generating Combined Analysis ==="
"$SCRIPT_DIR/analysis.sh"

echo ""
echo "=== All Benchmarks Complete ==="
echo "Results available in: $RESULTS_DIR"
echo ""
echo "Key files:"
echo "  - summary.txt: Binary benchmark results"
echo "  - docker_summary.txt: Docker image benchmark results" 
echo "  - analysis.txt: Combined analysis and recommendations"