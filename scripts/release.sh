#!/bin/bash

# Release script for sreootb
# Usage: ./scripts/release.sh [patch|minor|major|version]

set -e

# Colors for output
RED='\033[0;31m'
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

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if git is clean
if ! git diff-index --quiet HEAD --; then
    print_error "Git working directory is not clean. Please commit or stash changes."
    exit 1
fi

# Get current version from latest tag
CURRENT_VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
print_info "Current version: $CURRENT_VERSION"

# Parse version numbers
VERSION_REGEX="^v([0-9]+)\.([0-9]+)\.([0-9]+)(-.*)?$"
if [[ $CURRENT_VERSION =~ $VERSION_REGEX ]]; then
    MAJOR=${BASH_REMATCH[1]}
    MINOR=${BASH_REMATCH[2]}
    PATCH=${BASH_REMATCH[3]}
    SUFFIX=${BASH_REMATCH[4]}
else
    print_error "Invalid current version format: $CURRENT_VERSION"
    exit 1
fi

# Determine new version
BUMP_TYPE=${1:-patch}
case $BUMP_TYPE in
    patch)
        NEW_VERSION="v$MAJOR.$MINOR.$((PATCH + 1))"
        ;;
    minor)
        NEW_VERSION="v$MAJOR.$((MINOR + 1)).0"
        ;;
    major)
        NEW_VERSION="v$((MAJOR + 1)).0.0"
        ;;
    v*.*.*)
        NEW_VERSION=$BUMP_TYPE
        ;;
    *.*.*)
        NEW_VERSION="v$BUMP_TYPE"
        ;;
    *)
        print_error "Invalid bump type: $BUMP_TYPE"
        print_error "Usage: $0 [patch|minor|major|v1.2.3]"
        exit 1
        ;;
esac

print_info "New version will be: $NEW_VERSION"

# Test build with version information
print_info "Testing build with version information..."
COMMIT=$(git rev-parse HEAD)
DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

go build \
    -ldflags="-X github.com/x86txt/sreootb/cmd.Version=$NEW_VERSION -X github.com/x86txt/sreootb/cmd.Commit=$COMMIT -X github.com/x86txt/sreootb/cmd.Date=$DATE" \
    -o sreootb-test .

# Test the version output
print_info "Testing version output..."
VERSION_OUTPUT=$(./sreootb-test --version)
if [[ "$VERSION_OUTPUT" == "SREootb $NEW_VERSION" ]]; then
    print_info "✅ Version test passed: $VERSION_OUTPUT"
else
    print_error "❌ Version test failed. Expected 'SREootb $NEW_VERSION', got '$VERSION_OUTPUT'"
    rm -f sreootb-test
    exit 1
fi

# Clean up test binary
rm -f sreootb-test

# Confirm with user
read -p "Create release $NEW_VERSION? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    print_warning "Release cancelled."
    exit 0
fi

# Create and push tag
print_info "Creating tag: $NEW_VERSION"
git tag "$NEW_VERSION"

print_info "Pushing tag to origin..."
git push origin "$NEW_VERSION"

print_info "Release $NEW_VERSION created successfully!"
print_info "GitHub Actions will now build and create the release."
print_info "Check: https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^/]*\).*/\1/' | sed 's/\.git$//')/actions" 