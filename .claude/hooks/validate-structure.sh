#!/bin/bash
# goframe structure validation hook
# Validates Go files against goframe conventions before write
set -euo pipefail

FILE="${1:-}"
if [[ -z "$FILE" ]]; then
  exit 0
fi

if [[ "$FILE" != *.go ]]; then
  exit 0
fi

exec goframe check --file "$FILE" --output json
