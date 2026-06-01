#!/bin/bash
# Validate Version const matches service.yaml

REPO_DIR="${1:-.}"

# Read service.yaml version
if [ ! -f "$REPO_DIR/service.yaml" ]; then
  exit 0
fi

# Strip everything up to and including "version:", then any surrounding quotes
# and trailing whitespace. Avoids GNU-only sed extensions (\?) so it is portable
# across BSD/macOS and GNU sed.
SERVICE_VERSION=$(grep -E '^[[:space:]]*version:' "$REPO_DIR/service.yaml" | head -1 | sed -e 's/.*version:[[:space:]]*//' -e 's/["'"'"']//g' -e 's/[[:space:]]*$//')

if [ -z "$SERVICE_VERSION" ]; then
  exit 0
fi

# Check main.go
if [ ! -f "$REPO_DIR/main.go" ]; then
  exit 0
fi

CONST_VERSION=$(grep -E 'const\s+Version\s*=\s*"[^"]*"' "$REPO_DIR/main.go" | head -1 | sed 's/.*"\([^"]*\)".*/\1/')

if [ -z "$CONST_VERSION" ]; then
  exit 0
fi

# Compare
if [ "$SERVICE_VERSION" != "$CONST_VERSION" ]; then
  echo "ERROR: Version drift in $REPO_DIR"
  echo "  service.yaml: $SERVICE_VERSION"
  echo "  main.go const: $CONST_VERSION"
  exit 1
fi

exit 0
