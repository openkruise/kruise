#!/usr/bin/env python3
"""
Expression Generator for Piecewise Linear Functions

This tool generates mathematical expressions that represent piecewise linear functions
passing through given points.
"""

import argparse
import json
from typing import List, Tuple


def calculate_slope(p1: Tuple[float, float], p2: Tuple[float, float]) -> float:
    """Calculate the slope between two points."""
    x1, y1 = p1
    x2, y2 = p2
    if x2 == x1:
        raise ValueError(f"Points have same x-coordinate: {p1}, {p2}")
    return (y2 - y1) / (x2 - x1)


def line_equation(p1: Tuple[float, float], p2: Tuple[float, float], var: str) -> str:
    """Generate linear equation passing through two points."""
    slope = calculate_slope(p1, p2)
    x1, y1 = p1
    intercept = y1 - slope * x1

    # Format the equation nicely
    terms = []

    # Slope term
    if slope != 0:
        if slope == 1:
            terms.append(var)
        elif slope == -1:
            terms.append(f"-{var}")
        elif slope == int(slope):
            terms.append(f"{int(slope)}*{var}")
        else:
            terms.append(f"{slope}*{var}")

    # Intercept term
    if intercept != 0:
        if intercept > 0 and terms:
            terms.append(f"+{intercept if intercept == int(intercept) else intercept}")
        else:
            terms.append(f"{intercept if intercept == int(intercept) else intercept}")
    elif not terms:
        return "0"

    equation = "".join(str(t) for t in terms)
    # Clean up formatting
    equation = equation.replace("+-", "-").replace("+", "+").replace("*", "*")

    return equation


def generate_piecewise_expression(points: List[Tuple[float, float]], var: str = "x") -> str:
    """
    Generate a piecewise linear expression using max/min operators.

    Algorithm:
    For general piecewise linear functions, we need to construct expressions that:
    1. Are active (equal to y) exactly in the correct x range
    2. Use nested max/min to handle arbitrary slope changes
    """
    if len(points) < 2:
        raise ValueError("Need at least 2 points")

    # Sort points by x-coordinate
    points = sorted(points, key=lambda p: p[0])

    # Check if it's a simple straight line
    if len(points) == 2:
        return line_equation(points[0], points[1], var)

    # Check if all points are collinear
    slopes = []
    for i in range(len(points) - 1):
        slopes.append(calculate_slope(points[i], points[i + 1]))

    if len(set(slopes)) == 1:
        # All slopes are the same, return a simple line
        return line_equation(points[0], points[-1], var)

    # Generate piecewise expression
    segments = []
    for i in range(len(points) - 1):
        seg_expr = line_equation(points[i], points[i + 1], var)
        segments.append(seg_expr)

    # Check slope pattern
    slope_increasing = all(slopes[i] <= slopes[i + 1] for i in range(len(slopes) - 1))
    slope_decreasing = all(slopes[i] >= slopes[i + 1] for i in range(len(slopes) - 1))

    if slope_increasing:
        # Convex function - use max of all segments
        if len(segments) == 1:
            return segments[0]
        return f"max({','.join(segments)})"
    elif slope_decreasing:
        # Concave function - use min of all segments
        if len(segments) == 1:
            return segments[0]
        return f"min({','.join(segments)})"
    else:
        # Mixed slopes - need to build nested expression
        # Strategy: Build expression segment by segment using conditionals
        # For now, we'll use a combination approach that works for most cases

        # Try to split into convex and concave parts
        # Find inflection points (where slope changes from increasing to decreasing or vice versa)
        inflection_indices = []
        for i in range(len(slopes) - 1):
            if (slopes[i] < slopes[i + 1] and i > 0 and slopes[i - 1] > slopes[i]) or \
               (slopes[i] > slopes[i + 1] and i > 0 and slopes[i - 1] < slopes[i]):
                inflection_indices.append(i)

        # For complex cases, we construct a nested min/max expression
        # Build groups based on slope changes
        result = segments[0]
        for i in range(1, len(segments)):
            if slopes[i - 1] <= slopes[i]:
                # Increasing slope - use max
                result = f"max({result},{segments[i]})"
            else:
                # Decreasing slope - use min
                result = f"min({result},{segments[i]})"

        return result

def verify_expression(points: List[Tuple[float, float]], expression: str, var: str = "x") -> bool:
    """
    Verify that an expression passes through all given points.

    Args:
        points: List of (x, y) tuples
        expression: Mathematical expression string
        var: Variable name used in expression

    Returns:
        True if expression passes through all points, False otherwise
    """
    import sys

    for x_val, expected_y in points:
        # Create a namespace with the variable
        namespace = {var: x_val, 'max': max, 'min': min}

        try:
            actual_y = eval(expression, {"__builtins__": {}}, namespace)
            passed = abs(actual_y - expected_y) < 1e-10  # Account for floating point errors

            if not passed:
                print(f"Verification FAILED at {var}={x_val}: expected={expected_y}, actual={actual_y}", file=sys.stderr)
                return False
        except Exception as e:
            print(f"Verification ERROR at {var}={x_val}: {e}", file=sys.stderr)
            return False

    return True


def parse_points(points_str: str) -> List[Tuple[float, float]]:
    """Parse points from string representation."""
    try:
        # Try to parse as JSON array
        points_list = json.loads(points_str)
        return [(float(x), float(y)) for x, y in points_list]
    except json.JSONDecodeError:
        raise ValueError(f"Invalid points format: {points_str}")


def main():
    parser = argparse.ArgumentParser(
        description="Generate mathematical expression for piecewise linear function",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s "[[0,0], [1,1], [2,2], [3,3]]"
  %(prog)s "[[0,0], [1,1], [2,3]]" --var x
  %(prog)s "[[0,0], [1,1], [2,3]]" -v mem
        """
    )

    parser.add_argument(
        "points",
        type=str,
        help='Points as JSON array, e.g., "[[0,0], [1,1], [2,2]]"'
    )

    parser.add_argument(
        "-v", "--var",
        type=str,
        default="x",
        help="Variable name (default: x)"
    )

    parser.add_argument(
        "--no-verify",
        action="store_true",
        help="Skip automatic verification of generated expression"
    )

    args = parser.parse_args()

    try:
        points = parse_points(args.points)
        expression = generate_piecewise_expression(points, args.var)

        # Automatically verify the expression
        if not args.no_verify:
            if not verify_expression(points, expression, args.var):
                import sys
                print(f"\nWARNING: Generated expression failed verification!", file=sys.stderr)
                print(f"Expression: {expression}", file=sys.stderr)
                return 1

        print(expression)
    except Exception as e:
        parser.error(f"Error: {e}")
        return 1

    return 0


if __name__ == "__main__":
    exit(main())
