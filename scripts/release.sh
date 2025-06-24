#!/bin/bash

# Enhanced Release script for sreootb
# Usage: ./scripts/release.sh [patch|minor|major|version] [--dry-run]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
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

print_build() {
    echo -e "${BLUE}[BUILD]${NC} $1"
}

print_step() {
    echo -e "${BOLD}[STEP]${NC} $1"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to show help
show_help() {
    echo -e "${BOLD}SREootb Release Script${NC}"
    echo -e "======================"
    echo ""
    echo -e "${BOLD}USAGE:${NC}"
    echo -e "  ./scripts/release.sh [VERSION_BUMP] [OPTIONS]"
    echo ""
    echo -e "${BOLD}VERSION_BUMP:${NC}"
    echo -e "  ${GREEN}patch${NC}     Increment patch version (x.y.Z)"
    echo -e "  ${GREEN}minor${NC}     Increment minor version (x.Y.0)"
    echo -e "  ${GREEN}major${NC}     Increment major version (X.0.0)"
    echo -e "  ${GREEN}v1.2.3${NC}    Specific version number"
    echo ""
    echo -e "${BOLD}OPTIONS:${NC}"
    echo -e "  ${YELLOW}--dry-run${NC}   Show what would be done without making changes"
    echo -e "  ${YELLOW}--help${NC}      Show this help message"
    echo ""
    echo -e "${BOLD}EXAMPLES:${NC}"
    echo -e "  ./scripts/release.sh patch              # Release v1.0.1"
    echo -e "  ./scripts/release.sh minor              # Release v1.1.0"
    echo -e "  ./scripts/release.sh v2.0.0             # Release v2.0.0"
    echo -e "  ./scripts/release.sh patch --dry-run    # Test run"
    echo ""
    echo -e "${BOLD}REQUIREMENTS:${NC}"
    echo -e "  - GitHub CLI (gh) installed and authenticated"
    echo -e "  - Clean git working directory"
    echo -e "  - Push access to the repository"
    echo ""
    echo -e "${BOLD}PLATFORMS BUILT:${NC}"
    echo -e "  - Linux: amd64, arm64, arm, riscv64"
    echo -e "  - macOS: arm64"
    echo -e "  - Windows: amd64"
    echo ""
}

# Parse arguments
DRY_RUN=false
BUMP_TYPE="patch"

for arg in "$@"; do
    case $arg in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --help|-h)
            show_help
            exit 0
            ;;
        patch|minor|major|v*.*.*)
            BUMP_TYPE=$arg
            shift
            ;;
        *.*.*)
            BUMP_TYPE="v$arg"
            shift
            ;;
        *)
            print_error "Unknown argument: $arg"
            show_help
            exit 1
            ;;
    esac
done

# Print configuration
if [ "$DRY_RUN" = true ]; then
    print_warning "🧪 DRY RUN MODE - No changes will be made"
    echo ""
fi

print_step "1/7 Checking prerequisites"

# Check if GitHub CLI is installed
if ! command_exists gh; then
    print_error "GitHub CLI (gh) is not installed."
    print_info "Install it from: https://cli.github.com/"
    print_info "Or on Ubuntu/Debian: sudo apt install gh"
    print_info "Or on macOS: brew install gh"
    exit 1
fi

# Check if authenticated with GitHub
if ! gh auth status >/dev/null 2>&1; then
    print_error "Not authenticated with GitHub CLI."
    print_info "Run: gh auth login"
    exit 1
fi

# Check if git is clean
if ! git diff-index --quiet HEAD --; then
    print_error "Git working directory is not clean. Please commit or stash changes."
    if [ "$DRY_RUN" = false ]; then
        exit 1
    else
        print_warning "Continuing in dry-run mode..."
    fi
fi

# Check if on main/master branch
CURRENT_BRANCH=$(git branch --show-current)
if [[ "$CURRENT_BRANCH" != "main" && "$CURRENT_BRANCH" != "master" ]]; then
    print_warning "Not on main/master branch (currently on: $CURRENT_BRANCH)"
    if [ "$DRY_RUN" = false ]; then
        read -p "Continue anyway? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_warning "Release cancelled."
            exit 0
        fi
    fi
fi

print_step "2/7 Determining version"

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
    *)
        print_error "Invalid bump type: $BUMP_TYPE"
        show_help
        exit 1
        ;;
esac

print_info "New version will be: $NEW_VERSION"

# Check if tag already exists
if git tag -l | grep -q "^$NEW_VERSION$"; then
    print_error "Tag $NEW_VERSION already exists!"
    exit 1
fi

print_step "3/7 Testing build"

# Test build with version information
print_info "Testing build with version information..."
COMMIT=$(git rev-parse HEAD)
DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

if [ "$DRY_RUN" = false ]; then
    go build \
        -ldflags="-X github.com/x86txt/sreootb/cmd.Version=$NEW_VERSION -X github.com/x86txt/sreootb/cmd.Commit=$COMMIT -X github.com/x86txt/sreootb/cmd.Date=$DATE" \
        -o sreootb-test .

    # Test the version output
    print_info "Testing version output..."
    VERSION_OUTPUT=$(./sreootb-test --version 2>/dev/null || echo "version check failed")
    if [[ "$VERSION_OUTPUT" == *"$NEW_VERSION"* ]]; then
        print_info "✅ Version test passed: $VERSION_OUTPUT"
    else
        print_error "❌ Version test failed. Expected version '$NEW_VERSION' in output, got: '$VERSION_OUTPUT'"
        rm -f sreootb-test
        exit 1
    fi

    # Clean up test binary
    rm -f sreootb-test
else
    print_info "⏭️  Skipping build test in dry-run mode"
fi

print_step "4/7 User confirmation"

# Confirm with user
if [ "$DRY_RUN" = false ]; then
    echo ""
    print_info "🚀 Ready to create release $NEW_VERSION"
    print_info "This will:"
    print_info "  • Build binaries for all supported platforms"
    print_info "  • Create and push git tag $NEW_VERSION"
    print_info "  • Create GitHub release with release notes"
    print_info "  • Upload all built binaries as release assets"
    echo ""
    read -p "Create release $NEW_VERSION? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_warning "Release cancelled."
        exit 0
    fi
fi

print_step "5/7 Building binaries"

# Define platforms to build
PLATFORMS=(
    "linux-amd64"
    "linux-arm64" 
    "linux-arm"
    "linux-riscv64"
    "darwin-arm64"
    "windows-amd64"
)

# Create release directory
RELEASE_DIR="release-$NEW_VERSION"
if [ "$DRY_RUN" = false ]; then
    rm -rf "$RELEASE_DIR"
    mkdir -p "$RELEASE_DIR"
fi

print_info "Building for platforms: ${PLATFORMS[*]}"

if [ "$DRY_RUN" = false ]; then
    # Use the existing build script to build all platforms
    print_build "Building all release targets..."
    ./scripts/build.sh linux,darwin,windows all

    # Move binaries to release directory and rename them properly
    print_build "Preparing release assets..."
    
    # Find all built binaries
    for file in built/sreootb-*; do
        if [ -f "$file" ]; then
            # Copy to release directory
            cp "$file" "$RELEASE_DIR/"
            print_info "📦 Prepared: $(basename "$file")"
        fi
    done
    
    # Check if we have any binaries
    if [ -z "$(ls -A "$RELEASE_DIR" 2>/dev/null)" ]; then
        print_error "No binaries were built! Check the build process."
        exit 1
    fi
    
    print_info "✅ All binaries built successfully"
    ls -la "$RELEASE_DIR/"
else
    print_info "⏭️  Skipping binary build in dry-run mode"
fi

print_step "6/7 Creating GitHub release"

if [ "$DRY_RUN" = false ]; then
    # Create and push tag
    print_info "Creating tag: $NEW_VERSION"
    git tag "$NEW_VERSION"
    
    print_info "Pushing tag to origin..."
    git push origin "$NEW_VERSION"
    
    # Generate release notes
    RELEASE_NOTES_FILE=$(mktemp)
    cat > "$RELEASE_NOTES_FILE" << EOF
# SREootb $NEW_VERSION

## 🚀 What's New

Built on $(date -u +"%Y-%m-%d") from commit [\`${COMMIT:0:8}\`](https://github.com/$(gh repo view --json owner,name -q '.owner.login + "/" + .name')/commit/$COMMIT).

## 📦 Downloads

### Quick Start
Download the appropriate binary for your system and make it executable:

\`\`\`bash
# Linux x86_64
wget https://github.com/$(gh repo view --json owner,name -q '.owner.login + "/" + .name')/releases/download/$NEW_VERSION/sreootb-linux-amd64
chmod +x sreootb-linux-amd64
sudo mv sreootb-linux-amd64 /usr/local/bin/sreootb

# macOS Apple Silicon
wget https://github.com/$(gh repo view --json owner,name -q '.owner.login + "/" + .name')/releases/download/$NEW_VERSION/sreootb-darwin-arm64
chmod +x sreootb-darwin-arm64
sudo mv sreootb-darwin-arm64 /usr/local/bin/sreootb

# Run in standalone mode (server + local agent)
sreootb standalone
\`\`\`

### Available Platforms
- **Linux**: amd64, arm64, arm, riscv64
- **macOS**: arm64 (Apple Silicon)
- **Windows**: amd64

## 🔧 Usage

### Standalone Mode (Recommended)
Perfect for simple deployments - runs both server and local agent:
\`\`\`bash
sreootb standalone
# Web dashboard: https://localhost:8080
\`\`\`

### Server Mode
Run monitoring server only:
\`\`\`bash
sreootb server
\`\`\`

### Agent Mode  
Run monitoring agent only:
\`\`\`bash
sreootb agent --server-url https://your-server.com --api-key YOUR_KEY
\`\`\`

## 📖 Documentation
- [Main Documentation](https://github.com/$(gh repo view --json owner,name -q '.owner.login + "/" + .name')#readme)
- [Build Guide](https://github.com/$(gh repo view --json owner,name -q '.owner.login + "/" + .name')/blob/main/BUILD.md)
- [HA Setup Guide](https://github.com/$(gh repo view --json owner,name -q '.owner.login + "/" + .name')/blob/main/COCKROACHDB_HA_SETUP.md)

---
**Full Changelog**: https://github.com/$(gh repo view --json owner,name -q '.owner.login + "/" + .name')/compare/$CURRENT_VERSION...$NEW_VERSION
EOF

    # Create GitHub release
    print_info "Creating GitHub release..."
    gh release create "$NEW_VERSION" \
        --title "SREootb $NEW_VERSION" \
        --notes-file "$RELEASE_NOTES_FILE" \
        --latest

    # Clean up
    rm "$RELEASE_NOTES_FILE"
    
    print_info "✅ GitHub release created"
else
    print_info "⏭️  Skipping GitHub release creation in dry-run mode"
    print_info "Would create tag: $NEW_VERSION"
    print_info "Would push tag to origin"
    print_info "Would create GitHub release with generated release notes"
fi

print_step "7/7 Uploading release assets"

if [ "$DRY_RUN" = false ]; then
    # Upload all binaries as release assets
    print_info "Uploading release assets..."
    
    for file in "$RELEASE_DIR"/*; do
        if [ -f "$file" ]; then
            print_info "⬆️  Uploading $(basename "$file")..."
            gh release upload "$NEW_VERSION" "$file"
        fi
    done
    
    # Clean up release directory
    rm -rf "$RELEASE_DIR"
    
    print_info "✅ All assets uploaded successfully"
else
    print_info "⏭️  Skipping asset upload in dry-run mode"
    for platform in "${PLATFORMS[@]}"; do
        print_info "Would upload: sreootb-$platform"
    done
fi

# Final success message
echo ""
print_info "🎉 Release $NEW_VERSION completed successfully!"
echo ""

if [ "$DRY_RUN" = false ]; then
    REPO_URL=$(gh repo view --json url -q '.url')
    print_info "📋 Release Details:"
    print_info "  • Version: $NEW_VERSION"
    print_info "  • Commit: $COMMIT"
    print_info "  • Platforms: ${#PLATFORMS[@]} binaries"
    print_info "  • Release URL: $REPO_URL/releases/tag/$NEW_VERSION"
    echo ""
    print_info "🔗 Quick links:"
    print_info "  • View release: gh release view $NEW_VERSION --web"
    print_info "  • Download assets: gh release download $NEW_VERSION"
else
    print_info "🧪 Dry run completed - no changes were made"
    print_info "Run without --dry-run to create the actual release"
fi 