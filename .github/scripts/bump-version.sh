#!/usr/bin/env bash
# bump-version.sh — Calculate the next semver version.
#
# Usage: . bump-version.sh <latest-tag> <tag-prefix> <bump-type>
#   latest-tag:  e.g. "v0.1.0-beta.10" or "addons/charmvt/v0.1.0-beta.3" or "" (first release)
#   tag-prefix:  e.g. "" or "addons/charmvt/"
#   bump-type:   beta | patch | minor | major
#
# Prints the next version string (e.g. "v0.1.0-beta.11") to stdout.

LATEST="$1"
TAG_PREFIX="$2"
BUMP="$3"

if [ -z "$LATEST" ]; then
  if [ "$BUMP" = "beta" ]; then
    echo "v0.1.0-beta.1"
  else
    echo "v0.1.0"
  fi
  exit 0
fi

# Strip tag prefix and leading v
V="${LATEST#"$TAG_PREFIX"}"
V="${V#v}"

if echo "$V" | grep -q "beta"; then
  BASE=$(echo "$V" | sed 's/-beta\..*//')
  BETA_NUM=$(echo "$V" | sed 's/.*-beta\.//')

  case "$BUMP" in
    beta)
      echo "v${BASE}-beta.$((BETA_NUM + 1))"
      ;;
    patch)
      echo "v${BASE}"
      ;;
    minor)
      MAJOR=$(echo "$BASE" | cut -d. -f1)
      MINOR=$(echo "$BASE" | cut -d. -f2)
      echo "v${MAJOR}.$((MINOR + 1)).0"
      ;;
    major)
      MAJOR=$(echo "$BASE" | cut -d. -f1)
      echo "v$((MAJOR + 1)).0.0"
      ;;
  esac
else
  MAJOR=$(echo "$V" | cut -d. -f1)
  MINOR=$(echo "$V" | cut -d. -f2)
  PATCH=$(echo "$V" | cut -d. -f3)

  case "$BUMP" in
    beta)
      echo "v${MAJOR}.$((MINOR + 1)).0-beta.1"
      ;;
    patch)
      echo "v${MAJOR}.${MINOR}.$((PATCH + 1))"
      ;;
    minor)
      echo "v${MAJOR}.$((MINOR + 1)).0"
      ;;
    major)
      echo "v$((MAJOR + 1)).0.0"
      ;;
  esac
fi
