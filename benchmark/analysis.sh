#!/bin/bash

# Break-even analysis and recommendations for UPX usage
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="$SCRIPT_DIR/results"

echo "=== UPX Break-Even Analysis ==="

# Check if results exist
if [ ! -d "$RESULTS_DIR" ]; then
    echo "No results found. Please run benchmarks first."
    exit 1
fi

# Generate comprehensive analysis
cat > "$RESULTS_DIR/analysis.txt" << 'EOF'
UPX Impact Analysis for moby-ryuk
==================================

This analysis evaluates the trade-offs of using UPX compression for the Ryuk binary.

FACTORS ANALYZED:
1. Binary size reduction
2. Docker image size reduction  
3. Startup time overhead
4. Pull time improvement
5. Break-even scenarios

EOF

# Add binary results if available
if [ -f "$RESULTS_DIR/summary.txt" ]; then
    echo "BINARY ANALYSIS:" >> "$RESULTS_DIR/analysis.txt"
    cat "$RESULTS_DIR/summary.txt" >> "$RESULTS_DIR/analysis.txt"
    echo "" >> "$RESULTS_DIR/analysis.txt"
fi

# Add docker results if available
if [ -f "$RESULTS_DIR/docker_summary.txt" ]; then
    echo "DOCKER IMAGE ANALYSIS:" >> "$RESULTS_DIR/analysis.txt"
    cat "$RESULTS_DIR/docker_summary.txt" >> "$RESULTS_DIR/analysis.txt"
    echo "" >> "$RESULTS_DIR/analysis.txt"
fi

# Calculate break-even scenarios
cat >> "$RESULTS_DIR/analysis.txt" << 'EOF'
BREAK-EVEN ANALYSIS:
====================

The break-even point depends on usage patterns:

1. NETWORK-CONSTRAINED ENVIRONMENTS:
   - Slower internet connections benefit more from smaller images
   - Each MB saved in image size reduces pull time significantly
   - UPX compression is beneficial when pull time savings > startup overhead

2. STARTUP-CRITICAL APPLICATIONS:
   - Applications requiring very fast startup times may not benefit
   - Consider the frequency of container starts vs pulls
   - If containers start frequently but are pulled rarely, UPX may hurt performance

3. STORAGE-CONSTRAINED ENVIRONMENTS:
   - Container registries with storage costs benefit from smaller images
   - Local storage savings on nodes running many containers
   - Reduced bandwidth usage for image distribution

CALCULATION FRAMEWORK:
=====================

Break-even formula:
  (Pull Time Savings per Pull) × (Number of Pulls) > (Startup Overhead) × (Number of Starts)

Where:
- Pull Time Savings = (Baseline Pull Time - UPX Pull Time)  
- Startup Overhead = (UPX Startup Time - Baseline Startup Time)
- Frequency ratio = Pulls / Starts (typically < 1 in production)

RECOMMENDATIONS:
===============

EOF

# Add specific recommendations based on results
if [ -f "$RESULTS_DIR/size_mb_baseline.txt" ] && [ -f "$RESULTS_DIR/size_mb_upx.txt" ]; then
    baseline_size=$(cat "$RESULTS_DIR/size_mb_baseline.txt" 2>/dev/null || echo "0")
    upx_size=$(cat "$RESULTS_DIR/size_mb_upx.txt" 2>/dev/null || echo "0")
    
    if [ "$baseline_size" != "0" ] && [ "$upx_size" != "0" ]; then
        size_reduction=$(echo "scale=1; ($baseline_size - $upx_size) / $baseline_size * 100" | bc -l 2>/dev/null || echo "0")
        
        cat >> "$RESULTS_DIR/analysis.txt" << EOF
Based on measured size reduction of ${size_reduction}%:

EOF
        
        # Determine recommendation based on size reduction
        size_reduction_int=$(echo "$size_reduction" | cut -d. -f1)
        if [ "$size_reduction_int" -gt 40 ]; then
            cat >> "$RESULTS_DIR/analysis.txt" << 'EOF'
✅ RECOMMEND UPX: Significant size reduction (>40%) justifies startup overhead
   - Especially beneficial for CI/CD pipelines with frequent pulls
   - Network bandwidth savings are substantial
   - Consider enabling UPX by default

EOF
        elif [ "$size_reduction_int" -gt 20 ]; then
            cat >> "$RESULTS_DIR/analysis.txt" << 'EOF'
⚖️  CONDITIONAL RECOMMENDATION: Moderate size reduction (20-40%)
   - Beneficial for network-constrained environments
   - Consider making UPX optional via build argument
   - Test impact on your specific use case

EOF
        else
            cat >> "$RESULTS_DIR/analysis.txt" << 'EOF'
❌ DO NOT RECOMMEND: Small size reduction (<20%) may not justify overhead
   - Startup time impact likely outweighs benefits
   - Consider other optimization approaches first
   - UPX may not be worth the complexity

EOF
        fi
    fi
fi

# Add startup time analysis if available
if [ -f "$RESULTS_DIR/avg_startup_baseline.txt" ] && [ -f "$RESULTS_DIR/avg_startup_upx.txt" ]; then
    baseline_startup=$(cat "$RESULTS_DIR/avg_startup_baseline.txt" 2>/dev/null || echo "0")
    upx_startup=$(cat "$RESULTS_DIR/avg_startup_upx.txt" 2>/dev/null || echo "0")
    
    if [ "$baseline_startup" != "0" ] && [ "$upx_startup" != "0" ]; then
        startup_overhead=$(echo "scale=1; ($upx_startup - $baseline_startup) / $baseline_startup * 100" | bc -l 2>/dev/null || echo "0")
        
        cat >> "$RESULTS_DIR/analysis.txt" << EOF

STARTUP TIME IMPACT: ${startup_overhead}% overhead
EOF
        
        startup_overhead_int=$(echo "$startup_overhead" | cut -d. -f1)
        if [ "$startup_overhead_int" -gt 50 ]; then
            echo "⚠️  HIGH OVERHEAD: UPX significantly impacts startup time" >> "$RESULTS_DIR/analysis.txt"
        elif [ "$startup_overhead_int" -gt 20 ]; then
            echo "⚠️  MODERATE OVERHEAD: Consider impact on startup-critical workloads" >> "$RESULTS_DIR/analysis.txt"
        else
            echo "✅ LOW OVERHEAD: Startup impact is acceptable" >> "$RESULTS_DIR/analysis.txt"
        fi
    fi
fi

cat >> "$RESULTS_DIR/analysis.txt" << 'EOF'

IMPLEMENTATION RECOMMENDATIONS:
==============================

1. CONFIGURABLE UPX:
   - Add build argument to enable/disable UPX compression
   - Default to UPX disabled for broad compatibility
   - Provide UPX-enabled variant for size-optimized deployments

2. DOCUMENTATION:
   - Document trade-offs clearly in README
   - Provide benchmarking results
   - Include guidance for choosing between variants

3. CI/CD CONSIDERATIONS:
   - Build both variants in CI pipeline
   - Tag appropriately (e.g., :latest vs :latest-compact)
   - Monitor real-world performance impacts

4. FUTURE OPTIMIZATIONS:
   - Consider alternative compression methods
   - Explore static linking optimizations
   - Investigate Go build flag optimizations

EOF

echo "Analysis complete!"
echo ""
echo "=== Summary ==="
cat "$RESULTS_DIR/analysis.txt" | tail -n 30

echo ""
echo "Full analysis saved to: $RESULTS_DIR/analysis.txt"