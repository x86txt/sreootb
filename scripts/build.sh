#!/bin/bash

# Build script for local development
# Usage: ./scripts/build.sh [output-name]

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Set output binary name
OUTPUT_NAME=${1:-sreootb}

# Get version information
if git describe --tags --exact-match HEAD 2>/dev/null; then
    # If we're on a tagged commit, use the tag
    VERSION=$(git describe --tags --exact-match HEAD)
else
    # Otherwise, use dev version with short commit hash
    VERSION="dev-$(git rev-parse --short HEAD)"
fi

COMMIT=$(git rev-parse HEAD)
DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

print_info "Building SREootb..."
print_info "Version: $VERSION"
print_info "Commit:  $COMMIT"
print_info "Date:    $DATE"
print_info "Output:  $OUTPUT_NAME"

# Check if there are uncommitted changes
if ! git diff-index --quiet HEAD --; then
    print_warning "Building with uncommitted changes"
    VERSION="$VERSION-dirty"
fi

# Build the binary
go build \
    -ldflags="-X github.com/x86txt/sreootb/cmd.Version=$VERSION -X github.com/x86txt/sreootb/cmd.Commit=$COMMIT -X github.com/x86txt/sreootb/cmd.Date=$DATE" \
    -o "$OUTPUT_NAME" .

print_info "✅ Build complete: $OUTPUT_NAME"
print_info "Test with: ./$OUTPUT_NAME --version"

# Test the version output
VERSION_OUTPUT=$(./"$OUTPUT_NAME" --version)
print_info "Version output: $VERSION_OUTPUT" 