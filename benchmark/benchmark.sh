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
    local iterations=100
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
        
        # Print progress every 10 iterations
        if [ $((i % 10)) -eq 0 ]; then
            printf "  Progress: %d/%d iterations completed\n" "$i" "$iterations"
        fi
    done
    
    # Calculate comprehensive statistics
    local sorted_file="/tmp/sorted_$name.txt"
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
    
    # Save all statistics
    echo "$avg" > "$RESULTS_DIR/avg_startup_$name.txt"
    echo "$median" > "$RESULTS_DIR/median_startup_$name.txt"
    echo "$min" > "$RESULTS_DIR/min_startup_$name.txt"
    echo "$max" > "$RESULTS_DIR/max_startup_$name.txt"
    echo "$p90" > "$RESULTS_DIR/p90_startup_$name.txt"
    
    # Cleanup
    rm -f "$sorted_file"
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
echo "Startup Times (100 iterations):" >> "$RESULTS_DIR/summary.txt"

if [ -f "$RESULTS_DIR/avg_startup_baseline.txt" ]; then
    baseline_avg=$(cat "$RESULTS_DIR/avg_startup_baseline.txt")
    baseline_median=$(cat "$RESULTS_DIR/median_startup_baseline.txt")
    baseline_min=$(cat "$RESULTS_DIR/min_startup_baseline.txt")
    baseline_max=$(cat "$RESULTS_DIR/max_startup_baseline.txt")
    baseline_p90=$(cat "$RESULTS_DIR/p90_startup_baseline.txt")
    cat >> "$RESULTS_DIR/summary.txt" << EOF
  Baseline:
    Mean: ${baseline_avg} ms
    Median: ${baseline_median} ms
    Min: ${baseline_min} ms
    Max: ${baseline_max} ms
    90th percentile: ${baseline_p90} ms
EOF
fi

if [ -f "$RESULTS_DIR/avg_startup_optimized.txt" ]; then
    optimized_avg=$(cat "$RESULTS_DIR/avg_startup_optimized.txt")
    optimized_median=$(cat "$RESULTS_DIR/median_startup_optimized.txt")
    optimized_min=$(cat "$RESULTS_DIR/min_startup_optimized.txt")
    optimized_max=$(cat "$RESULTS_DIR/max_startup_optimized.txt")
    optimized_p90=$(cat "$RESULTS_DIR/p90_startup_optimized.txt")
    cat >> "$RESULTS_DIR/summary.txt" << EOF
  Optimized:
    Mean: ${optimized_avg} ms
    Median: ${optimized_median} ms
    Min: ${optimized_min} ms
    Max: ${optimized_max} ms
    90th percentile: ${optimized_p90} ms
EOF
fi

if [ -f "$RESULTS_DIR/avg_startup_upx.txt" ]; then
    upx_avg=$(cat "$RESULTS_DIR/avg_startup_upx.txt")
    upx_median=$(cat "$RESULTS_DIR/median_startup_upx.txt")
    upx_min=$(cat "$RESULTS_DIR/min_startup_upx.txt")
    upx_max=$(cat "$RESULTS_DIR/max_startup_upx.txt")
    upx_p90=$(cat "$RESULTS_DIR/p90_startup_upx.txt")
    cat >> "$RESULTS_DIR/summary.txt" << EOF
  UPX:
    Mean: ${upx_avg} ms
    Median: ${upx_median} ms
    Min: ${upx_min} ms
    Max: ${upx_max} ms
    90th percentile: ${upx_p90} ms
EOF
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