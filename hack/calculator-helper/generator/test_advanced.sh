#!/bin/bash

# Advanced test script for expression generator

echo "=========================================="
echo "Advanced Expression Generator Tests"
echo "=========================================="
echo ""

test_case() {
    local name="$1"
    local points="$2"
    echo "----------------------------------------"
    echo "Test: $name"
    echo "Points: $points"
    echo -n "Expression: "
    result=$(python3 generate_expression.py "$points" 2>&1)
    exit_code=$?
    if [ $exit_code -eq 0 ]; then
        echo "$result"
        echo "Status: ✓ PASSED (with auto-verification)"
    else
        echo ""
        echo "$result"
        echo "Status: ✗ FAILED"
    fi
    echo ""
}

# Test 1: Simple linear
test_case "Linear function" "[[0,0],[1,1],[2,2],[3,3]]"

# Test 2: Convex (increasing slopes)
test_case "Convex function" "[[0,0],[1,1],[2,3]]"

# Test 3: Concave (decreasing slopes)
test_case "Concave function" "[[0,10],[5,20],[10,15]]"

# Test 4: Mixed slopes (alternating)
test_case "Mixed slopes (up-down-up)" "[[0,0],[1,2],[2,1],[3,3]]"

# Test 5: Very complex case
test_case "Complex zigzag" "[[0,0],[1,3],[2,1],[3,4],[4,2]]"

# Test 6: Custom variable
echo "----------------------------------------"
echo "Test: Custom variable name"
echo "Points: [[0,0],[1,1],[2,3]]"
echo -n "Expression with var='mem': "
python3 generate_expression.py "[[0,0],[1,1],[2,3]]" --var mem 2>&1
echo ""

# Test 7: Negative values
test_case "Negative values" "[[-2,-4],[-1,-1],[0,0],[1,1]]"

# Test 8: Large values
test_case "Large values" "[[0,100],[10,200],[20,150]]"

echo "=========================================="
echo "All advanced tests completed!"
echo "=========================================="
