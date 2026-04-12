#!/usr/bin/env bash
set -euo pipefail

MIN_COVERAGE=90

# Find Go packages with staged changes
changed_files=$(git diff --cached --name-only --diff-filter=ACM -- '*.go' | grep -v '_test.go$' || true)

if [ -z "$changed_files" ]; then
    exit 0
fi

# Extract unique package directories
packages=$(echo "$changed_files" | xargs -I{} dirname {} | sort -u)

failed=0

for pkg in $packages; do
    # Skip if no test files exist in the package
    if ! ls "$pkg"/*_test.go &>/dev/null; then
        echo "FAIL: $pkg — no test files found (every .go file needs a _test.go)"
        failed=1
        continue
    fi

    # Run coverage for the package
    output=$(go test -cover "./$pkg" 2>&1) || {
        echo "FAIL: $pkg — tests failed"
        echo "$output"
        failed=1
        continue
    }

    # Extract coverage percentage
    coverage=$(echo "$output" | grep -oE '[0-9]+\.[0-9]+%' | head -1 | tr -d '%')

    if [ -z "$coverage" ]; then
        echo "FAIL: $pkg — could not determine coverage"
        failed=1
        continue
    fi

    # Compare using awk (handles floats)
    below=$(awk "BEGIN { print ($coverage < $MIN_COVERAGE) }")

    if [ "$below" = "1" ]; then
        echo "FAIL: $pkg — coverage ${coverage}% < ${MIN_COVERAGE}%"
        failed=1
    else
        echo "  OK: $pkg — coverage ${coverage}%"
    fi
done

if [ "$failed" -ne 0 ]; then
    echo ""
    echo "Coverage check failed. Minimum required: ${MIN_COVERAGE}%"
    exit 1
fi
