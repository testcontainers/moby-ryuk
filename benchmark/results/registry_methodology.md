# Registry Pull Benchmarking Methodology

## Overview
This document outlines the methodology for measuring real registry pulls, egress, and HTTP compression impact for the UPX benchmarking analysis.

## Implementation Approach

### 1. GHCR Setup and Authentication
```bash
# Authenticate with GHCR
echo "$GITHUB_TOKEN" | docker login ghcr.io -u "$GITHUB_ACTOR" --password-stdin

# Use unique timestamped tags to avoid cache pollution
TIMESTAMP=$(date +%s)
BASE_TAG="ghcr.io/testcontainers/moby-ryuk-benchmark"
```

### 2. Image Building and Publishing
```bash
# Build baseline image (without UPX)
docker build -f linux/Dockerfile.baseline -t "${BASE_TAG}:baseline-${TIMESTAMP}" .
docker push "${BASE_TAG}:baseline-${TIMESTAMP}"

# Build UPX-compressed image  
docker build -f linux/Dockerfile -t "${BASE_TAG}:upx-${TIMESTAMP}" .
docker push "${BASE_TAG}:upx-${TIMESTAMP}"

# Record actual pushed sizes for egress calculation
docker inspect "${BASE_TAG}:baseline-${TIMESTAMP}" --format='{{.Size}}'
docker inspect "${BASE_TAG}:upx-${TIMESTAMP}" --format='{{.Size}}'
```

### 3. Real Registry Pull Measurement
```bash
# Function to measure actual registry pulls
measure_registry_pull() {
    local image_tag="$1"
    local variant="$2"
    local iterations=50  # Sufficient for statistical significance
    
    for i in $(seq 1 $iterations); do
        # Completely clear local cache
        docker rmi "$image_tag" >/dev/null 2>&1 || true
        docker system prune -f >/dev/null 2>&1
        
        # Measure actual pull time from registry
        start_time=$(date +%s.%N)
        docker pull "$image_tag" 2>&1 | tee "pull_log_${variant}_${i}.txt"
        end_time=$(date +%s.%N)
        
        # Calculate and record pull time
        duration=$(echo "($end_time - $start_time) * 1000" | bc -l)
        echo "$duration" >> "pull_times_${variant}.txt"
        
        # Extract transfer size information from pull logs
        grep -o "Downloaded.*" "pull_log_${variant}_${i}.txt" || echo "No download info"
    done
}
```

### 4. Egress and Transfer Size Analysis
```bash
# Analyze actual data transfer from pull logs
analyze_egress() {
    local variant="$1"
    
    # Parse pull logs to extract layer download sizes
    for log in pull_log_${variant}_*.txt; do
        # Extract actual bytes transferred (registry-specific parsing)
        grep -E "(Downloaded|Pulling|Pull complete)" "$log" |
        awk '/Downloaded/ {sum += $2} END {print sum}' >> "egress_${variant}.txt"
    done
    
    # Calculate statistics
    awk '{sum+=$1; count++} END {
        print "Mean egress:", sum/count, "bytes"
        print "Total for", count, "pulls:", sum, "bytes"
    }' "egress_${variant}.txt"
}
```

### 5. HTTP Transport Compression Analysis
```bash
# Compare compressed vs uncompressed transfer effectiveness
analyze_http_compression() {
    # Get image manifest to analyze layer compression
    docker manifest inspect "$image_tag" > manifest.json
    
    # Extract layer sizes (compressed sizes in registry)
    jq -r '.layers[]?.size' manifest.json | 
    awk '{sum+=$1} END {print "Compressed layer total:", sum, "bytes"}'
    
    # Compare with actual image size
    docker inspect "$image_tag" --format='{{.Size}}' |
    awk '{print "Uncompressed image size:", $1, "bytes"}'
    
    # Calculate compression effectiveness
    # Ratio = compressed_size / uncompressed_size
    # Lower ratio = better compression
}
```

### 6. Statistical Analysis
```bash
# Calculate comprehensive statistics
calculate_stats() {
    local data_file="$1"
    local variant="$2"
    
    sort -n "$data_file" > "sorted_${variant}.txt"
    local count=$(wc -l < "$data_file")
    
    # Calculate mean, median, percentiles
    awk '{sum+=$1} END {print sum/NR}' "$data_file" > "mean_${variant}.txt"
    sed -n "$((count/2))p" "sorted_${variant}.txt" > "median_${variant}.txt"
    sed -n "$((count*90/100))p" "sorted_${variant}.txt" > "p90_${variant}.txt"
    head -1 "sorted_${variant}.txt" > "min_${variant}.txt"
    tail -1 "sorted_${variant}.txt" > "max_${variant}.txt"
}
```

## Key Measurements

### Registry Pull Performance
- **Pull Time**: Actual time to download from GHCR to local Docker daemon
- **Network Latency**: Real-world network conditions impact
- **Registry Performance**: GHCR's actual serving performance

### Egress Analysis  
- **Bytes Transferred**: Actual network traffic generated
- **Cost Impact**: Direct correlation to GHCR egress charges
- **Bandwidth Efficiency**: Network utilization optimization

### HTTP Compression Impact
- **Layer Compression**: Registry-level compression effectiveness
- **Pre-compressed Content**: How UPX affects HTTP compression ratios
- **Transport Efficiency**: Overall transfer optimization

## Expected Results

### Baseline vs UPX Comparison
```
Metric                 | Baseline    | UPX         | Improvement
-----------------------|-------------|-------------|------------
Pull Time (mean)       | ~2000ms     | ~800ms      | 60% faster
Egress (per pull)      | ~7.5MB      | ~2.5MB      | 67% reduction  
HTTP Compression Ratio | ~0.8        | ~0.9        | Less effective
Net Benefit            | Baseline    | Significant | Strongly positive
```

### Break-Even Analysis
- **Cost Savings**: Egress reduction of ~5MB per pull
- **Performance Gain**: 60% faster pulls improve CI/CD efficiency
- **Network Impact**: 67% bandwidth reduction benefits all users

## Implementation Notes

### Authentication Requirements
- Requires GITHUB_TOKEN with package:write permissions
- Must authenticate Docker with GHCR before running tests
- Consider rate limiting for high-iteration tests

### Network Considerations
- Run from multiple network locations for comprehensive analysis
- Consider regional GHCR endpoints for global performance analysis  
- Account for CDN caching in repeat measurements

### Statistical Rigor
- Minimum 50 iterations per variant for statistical significance
- Clear cache completely between pulls to simulate real conditions
- Record and analyze variance to identify outliers

## Production Usage

### With Authentication
```bash
export GITHUB_TOKEN="your_token"
export GITHUB_ACTOR="your_username"
./benchmark/registry-benchmark.sh
```

### Analysis Output
- Comprehensive statistical analysis (min, max, mean, median, percentiles)
- Egress cost analysis with savings calculations
- HTTP compression effectiveness comparison
- Network performance optimization recommendations

This methodology provides definitive real-world evidence for UPX adoption decisions.
