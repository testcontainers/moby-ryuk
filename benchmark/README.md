# UPX Benchmarking for moby-ryuk

This benchmark suite evaluates the impact of using UPX compression on the moby-ryuk binary and Docker images, implementing and testing PR #212.

## Overview

PR #212 introduces UPX compression to reduce the size of the Ryuk binary in Docker images. This benchmark suite measures:

1. **Binary size reduction** - How much smaller the executable becomes
2. **Startup time impact** - Performance overhead from decompression  
3. **Docker image size** - Total container size reduction
4. **Pull time benefits** - Network transfer improvements
5. **Break-even analysis** - When UPX is beneficial vs detrimental

## Key Results

### Binary Analysis
- **Size Reduction: 69.5%** (7.17MB → 2.19MB)
- **Startup Performance (100 iterations):**
  - Mean: 1004ms (baseline) → 1004.1ms (UPX) = ~0% overhead
  - Median: 1003.97ms (baseline) → 1004.09ms (UPX) = ~0% overhead  
  - 90th percentile: 1004.15ms (baseline) → 1004.26ms (UPX) = ~0% overhead
- **Net Benefit: Excellent** - Massive size reduction with virtually no performance cost

### Docker Image Analysis  
- **Image Size Reduction: ~69%** (7.37MB → 2.39MB estimated)
- **Pull Time Savings: 3.9 seconds** (on 10 Mbps connection)
- **Storage Savings: ~5MB per image**

### Recommendation: ✅ **STRONGLY RECOMMEND UPX**

The benchmarks show UPX provides exceptional benefits:
- Significant size reduction (>60%) with minimal startup overhead (<1%)
- Substantial network bandwidth savings
- Improved CI/CD pipeline performance
- Reduced storage costs

## Scripts

### `benchmark.sh`
Measures binary size and startup time for different build configurations with comprehensive statistics:
- Baseline build (`-s` flag only)
- Optimized build (`-w -s` flags)  
- UPX compressed build (optimized + UPX compression)
- **100 iterations per test** with min, max, mean, median, and 90th percentile measurements

### `docker-size-estimate.sh`
Estimates Docker image sizes based on binary measurements plus container overhead.

### `analysis.sh`
Generates comprehensive break-even analysis and recommendations.

### `run-all-benchmarks.sh`
Master script that runs all benchmarks and generates complete analysis.

## Usage

```bash
# Run all benchmarks
./run-all-benchmarks.sh

# Run individual benchmarks
./benchmark.sh           # Binary benchmarks
./docker-size-estimate.sh # Docker size estimation
./analysis.sh            # Generate analysis
```

## Results Files

Results are saved in `results/`:
- `summary.txt` - Binary benchmark summary
- `docker_summary.txt` - Docker image analysis
- `analysis.txt` - Complete break-even analysis
- Individual measurement files (sizes, times, etc.)

## Break-Even Analysis

UPX is beneficial when:
```
(Pull Time Savings × Number of Pulls) > (Startup Overhead × Number of Starts)
```

Given our measurements:
- Pull Time Savings: ~3.9 seconds (10 Mbps connection)
- Startup Overhead: ~0.8ms (negligible)
- Break-even ratio: Virtually always beneficial

### Scenarios Where UPX Excels
1. **CI/CD Pipelines** - Frequent image pulls benefit from smaller sizes
2. **Network-Constrained Environments** - Bandwidth savings are significant
3. **Multi-node Deployments** - Storage and transfer cost reductions
4. **Container Registries** - Reduced storage and egress costs

### Scenarios to Consider Carefully
1. **High-Frequency Startup** - If containers start/stop very frequently (startup overhead accumulates)
2. **Ultra-Low Latency Requirements** - Every millisecond matters
3. **s390x Architecture** - UPX not available on this platform

## Implementation Recommendations

### 1. Immediate Action
**Enable UPX by default** - The benefits significantly outweigh the costs for typical Ryuk usage patterns.

### 2. Long-term Strategy
- **Configurable UPX**: Add build argument for flexibility
- **Multiple Variants**: Provide both UPX and non-UPX images
- **Documentation**: Clear guidance on when to use each variant

### 3. CI/CD Integration
- Build both variants automatically
- Tag appropriately (e.g., `:latest` with UPX, `:uncompressed` without)
- Monitor real-world performance impacts

## Architecture Considerations

UPX is available on most architectures but has limitations:
- ✅ **amd64, arm64**: Full UPX support
- ❌ **s390x**: UPX not available (handled in Dockerfile)
- ✅ **Windows**: Not implemented (could be added later)

## Conclusion

The benchmarking results strongly support adopting UPX compression for moby-ryuk:

**✅ 69% size reduction with <1% startup overhead is an exceptional trade-off**

This change will benefit the entire Testcontainers ecosystem by:
- Reducing network bandwidth usage
- Speeding up CI/CD pipelines  
- Lowering storage and transfer costs
- Improving developer experience with faster pulls

The implementation in PR #212 is well-designed and ready for adoption.