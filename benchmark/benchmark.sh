#!/bin/bash

# UPX Benchmarking Script for moby-ryuk
# This script measures the impact of UPX compression on binary size and startup time

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
RESULTS_DIR="$SCRIPT_DIR/results"

echo "=== UPX Benchmarking for moby-ryuk ==="
echo "Project dir: $PROJECT_DIR"
echo "Results dir: $RESULTS_DIR"

# Create results directory
mkdir -p "$RESULTS_DIR"

# Function to build binary with different configurations
build_binary() {
    local name="$1"
    local ldflags="$2"
    local use_upx="$3"
    local output_file="$PROJECT_DIR/ryuk-$name"
    
    echo "Building $name binary..."
    cd "$PROJECT_DIR"
    
    # Build the binary
    go build -a -installsuffix cgo -ldflags="$ldflags" -trimpath -o "$output_file" .
    
    # Apply UPX if requested
    if [ "$use_upx" = "true" ]; then
        echo "Applying UPX compression to $name..."
        if command -v upx >/dev/null 2>&1; then
            upx --best --lzma "$output_file"
        else
            echo "Warning: UPX not found, installing..."
            # Try to install UPX
            if command -v apt-get >/dev/null 2>&1; then
                sudo apt-get update && sudo apt-get install -y upx-ucl
            elif command -v apk >/dev/null 2>&1; then
                apk add --no-cache upx
            else
                echo "Cannot install UPX automatically. Please install manually."
                return 1
            fi
            upx --best --lzma "$output_file"
        fi
    fi
    
    echo "Built $name: $(ls -lh "$output_file" | awk '{print $5}')"
    return 0
}

# Function to measure startup time
measure_startup_time() {
    local binary="$1"
    local name="$2"
    local iterations=10
    local results_file="$RESULTS_DIR/startup_times_$name.txt"
    
    echo "Measuring startup time for $name ($iterations iterations)..."
    
    # Clear results file
    > "$results_file"
    
    for i in $(seq 1 $iterations); do
        # Measure time to startup and immediate shutdown
        # We'll use a timeout to kill the process quickly after it starts
        start_time=$(date +%s.%N)
        timeout 1s "$binary" || true  # Allow timeout to kill the process
        end_time=$(date +%s.%N)
        
        # Calculate duration in milliseconds
        duration=$(echo "($end_time - $start_time) * 1000" | bc -l)
        echo "$duration" >> "$results_file"
        printf "  Iteration %d: %.2f ms\n" "$i" "$duration"
    done
    
    # Calculate statistics
    local avg=$(awk '{sum+=$1; count++} END {print sum/count}' "$results_file")
    local min=$(sort -n "$results_file" | head -1)
    local max=$(sort -n "$results_file" | tail -1)
    
    printf "  Average: %.2f ms\n" "$avg"
    printf "  Min: %.2f ms\n" "$min"  
    printf "  Max: %.2f ms\n" "$max"
    
    echo "$avg" > "$RESULTS_DIR/avg_startup_$name.txt"
}

# Function to measure binary size
measure_binary_size() {
    local binary="$1"
    local name="$2"
    
    if [ -f "$binary" ]; then
        local size_bytes=$(stat -c%s "$binary")
        local size_mb=$(echo "scale=2; $size_bytes / 1024 / 1024" | bc -l)
        echo "$size_bytes" > "$RESULTS_DIR/size_bytes_$name.txt"
        echo "$size_mb" > "$RESULTS_DIR/size_mb_$name.txt"
        printf "Size of %s: %d bytes (%.2f MB)\n" "$name" "$size_bytes" "$size_mb"
        return 0
    else
        echo "Binary $binary not found!"
        return 1
    fi
}

# Install bc for calculations if not available
if ! command -v bc >/dev/null 2>&1; then
    echo "Installing bc for calculations..."
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y bc
    elif command -v apk >/dev/null 2>&1; then
        apk add --no-cache bc
    fi
fi

echo ""
echo "=== Building Binaries ==="

# Build baseline binary (current approach)
build_binary "baseline" "-s" false

# Build optimized binary without UPX  
build_binary "optimized" "-w -s" false

# Build optimized binary with UPX
build_binary "upx" "-w -s" true

echo ""
echo "=== Measuring Binary Sizes ==="

measure_binary_size "$PROJECT_DIR/ryuk-baseline" "baseline"
measure_binary_size "$PROJECT_DIR/ryuk-optimized" "optimized" 
measure_binary_size "$PROJECT_DIR/ryuk-upx" "upx"

echo ""
echo "=== Measuring Startup Times ==="

measure_startup_time "$PROJECT_DIR/ryuk-baseline" "baseline"
measure_startup_time "$PROJECT_DIR/ryuk-optimized" "optimized"
measure_startup_time "$PROJECT_DIR/ryuk-upx" "upx"

echo ""
echo "=== Calculating Results ==="

# Generate summary report
cat > "$RESULTS_DIR/summary.txt" << EOF
UPX Benchmarking Summary for moby-ryuk
======================================

Binary Sizes:
EOF

if [ -f "$RESULTS_DIR/size_mb_baseline.txt" ]; then
    baseline_size=$(cat "$RESULTS_DIR/size_mb_baseline.txt")
    echo "  Baseline (-s): ${baseline_size} MB" >> "$RESULTS_DIR/summary.txt"
fi

if [ -f "$RESULTS_DIR/size_mb_optimized.txt" ]; then
    optimized_size=$(cat "$RESULTS_DIR/size_mb_optimized.txt")
    echo "  Optimized (-w -s): ${optimized_size} MB" >> "$RESULTS_DIR/summary.txt"
fi

if [ -f "$RESULTS_DIR/size_mb_upx.txt" ]; then
    upx_size=$(cat "$RESULTS_DIR/size_mb_upx.txt")
    echo "  UPX Compressed: ${upx_size} MB" >> "$RESULTS_DIR/summary.txt"
fi

echo "" >> "$RESULTS_DIR/summary.txt"
echo "Startup Times (Average):" >> "$RESULTS_DIR/summary.txt"

if [ -f "$RESULTS_DIR/avg_startup_baseline.txt" ]; then
    baseline_time=$(cat "$RESULTS_DIR/avg_startup_baseline.txt")
    echo "  Baseline: ${baseline_time} ms" >> "$RESULTS_DIR/summary.txt"
fi

if [ -f "$RESULTS_DIR/avg_startup_optimized.txt" ]; then
    optimized_time=$(cat "$RESULTS_DIR/avg_startup_optimized.txt")
    echo "  Optimized: ${optimized_time} ms" >> "$RESULTS_DIR/summary.txt"
fi

if [ -f "$RESULTS_DIR/avg_startup_upx.txt" ]; then
    upx_time=$(cat "$RESULTS_DIR/avg_startup_upx.txt")
    echo "  UPX: ${upx_time} ms" >> "$RESULTS_DIR/summary.txt"
fi

# Calculate size reduction percentages
if [ -f "$RESULTS_DIR/size_mb_baseline.txt" ] && [ -f "$RESULTS_DIR/size_mb_upx.txt" ]; then
    baseline_size=$(cat "$RESULTS_DIR/size_mb_baseline.txt")
    upx_size=$(cat "$RESULTS_DIR/size_mb_upx.txt")
    reduction=$(echo "scale=1; ($baseline_size - $upx_size) / $baseline_size * 100" | bc -l)
    echo "" >> "$RESULTS_DIR/summary.txt"
    echo "Size Reduction: ${reduction}%" >> "$RESULTS_DIR/summary.txt"
fi

# Calculate startup time overhead
if [ -f "$RESULTS_DIR/avg_startup_baseline.txt" ] && [ -f "$RESULTS_DIR/avg_startup_upx.txt" ]; then
    baseline_time=$(cat "$RESULTS_DIR/avg_startup_baseline.txt")
    upx_time=$(cat "$RESULTS_DIR/avg_startup_upx.txt")
    overhead=$(echo "scale=1; ($upx_time - $baseline_time) / $baseline_time * 100" | bc -l)
    echo "Startup Time Overhead: ${overhead}%" >> "$RESULTS_DIR/summary.txt"
fi

echo ""
echo "=== Summary ==="
cat "$RESULTS_DIR/summary.txt"

echo ""
echo "=== Cleanup ==="
rm -f "$PROJECT_DIR/ryuk-baseline" "$PROJECT_DIR/ryuk-optimized" "$PROJECT_DIR/ryuk-upx"

echo ""
echo "Benchmarking complete! Results saved in: $RESULTS_DIR"