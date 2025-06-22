#!/bin/bash

# Build script for local development and release targets
# Usage: ./scripts/build.sh <os> <target>
#   os: linux, darwin, windows (or comma-separated list)
#   target: amd64, arm64, arm, riscv64, all (or comma-separated list)

set -e

# Colors for output - minimal palette
GRAY='\033[0;37m'
DARK_GRAY='\033[1;30m'
BRIGHT_GREEN='\033[1;32m'
ORANGE='\033[38;5;208m'
RED='\033[0;31m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${BRIGHT_GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${RED}[WARNING]${NC} $1"
}

print_build() {
    echo -e "${BLUE}[BUILD]${NC} $1"
}

print_help() {
    echo -e "${BOLD}SREootb Build Script${NC}"
    echo -e "${DARK_GRAY}=====================${NC}"
    echo ""
    echo -e "${BOLD}USAGE:${NC}"
    echo -e "  ${GRAY}./scripts/build.sh${NC} ${DARK_GRAY}<OS> <TARGET>${NC}"
    echo ""
    echo -e "${BOLD}OPERATING SYSTEMS:${NC}"
    echo -e "  ${ORANGE}linux${NC}     Linux distributions"
    echo -e "  ${ORANGE}darwin${NC}    macOS (Apple Silicon)"
    echo -e "  ${ORANGE}windows${NC}   Microsoft Windows"
    echo ""
    echo -e "${BOLD}TARGETS:${NC}"
    echo -e "  ${BRIGHT_GREEN}amd64${NC}     x86_64 architecture"
    echo -e "  ${BRIGHT_GREEN}arm64${NC}     ARM 64-bit"
    echo -e "  ${BRIGHT_GREEN}arm${NC}       ARM 32-bit (Linux only)"
    echo -e "  ${BRIGHT_GREEN}riscv64${NC}   RISC-V 64-bit (Linux only)"
    echo -e "  ${BRIGHT_GREEN}all${NC}       All available targets for specified OS"
    echo ""
    echo -e "${BOLD}EXAMPLES:${NC}"
    echo -e "  ${GRAY}./scripts/build.sh linux amd64${NC}           # Linux x86_64"
    echo -e "  ${GRAY}./scripts/build.sh darwin arm64${NC}          # macOS ARM64"
    echo -e "  ${GRAY}./scripts/build.sh windows amd64${NC}         # Windows x86_64"
    echo -e "  ${GRAY}./scripts/build.sh linux all${NC}            # All Linux targets"
    echo -e "  ${GRAY}./scripts/build.sh linux,darwin amd64${NC}   # Linux & macOS x86_64"
    echo -e "  ${GRAY}./scripts/build.sh linux arm64,riscv64${NC}  # Linux ARM64 & RISC-V"
    echo ""
    echo -e "${BOLD}AVAILABLE COMBINATIONS:${NC}"
    echo -e "  ${ORANGE}Linux:${NC}    amd64, arm64, arm, riscv64"
    echo -e "  ${ORANGE}macOS:${NC}    arm64"
    echo -e "  ${ORANGE}Windows:${NC}  amd64"
    echo ""
    echo -e "${BOLD}OUTPUT:${NC}"
    echo -e "  ${DARK_GRAY}Single target:${NC} Creates binary in current directory"
    echo -e "  ${DARK_GRAY}Multiple targets:${NC} Creates binaries in ${GRAY}dist/${NC} directory"
    echo ""
}

# Parse arguments - show help if no arguments provided
if [ $# -eq 0 ]; then
    print_help
    exit 0
fi

OS_ARG=$1
TARGET_ARG=${2:-""}

# Handle help explicitly
if [ "$OS_ARG" = "help" ] || [ "$OS_ARG" = "--help" ] || [ "$OS_ARG" = "-h" ]; then
    print_help
    exit 0
fi

# Validate arguments
if [ -z "$TARGET_ARG" ]; then
    echo -e "${RED}Error: Target architecture required${NC}"
    echo ""
    print_help
    exit 1
fi

# Define available targets for each OS
declare -A OS_TARGETS=(
    ["linux"]="amd64 arm64 arm riscv64"
    ["darwin"]="arm64"
    ["windows"]="amd64"
)

# Function to expand "all" targets for an OS
expand_targets() {
    local os=$1
    local targets=$2
    
    if [ "$targets" = "all" ]; then
        echo "${OS_TARGETS[$os]}"
    else
        echo "$targets"
    fi
}

# Function to validate OS and target combination
validate_combination() {
    local os=$1
    local target=$2
    
    if [[ ! " ${!OS_TARGETS[@]} " =~ " $os " ]]; then
        echo -e "${RED}Error: Unsupported OS '$os'${NC}"
        echo "Supported: ${!OS_TARGETS[@]}"
        return 1
    fi
    
    local available_targets="${OS_TARGETS[$os]}"
    if [[ ! " $available_targets " =~ " $target " ]]; then
        echo -e "${RED}Error: Unsupported target '$target' for OS '$os'${NC}"
        echo "Available for $os: $available_targets"
        return 1
    fi
    
    return 0
}

# Parse comma-separated OS list
IFS=',' read -ra OS_LIST <<< "$OS_ARG"
# Parse comma-separated target list  
IFS=',' read -ra TARGET_LIST <<< "$TARGET_ARG"

# Build list of targets to compile
BUILD_TARGETS=()

# Get version information
if git describe --tags --exact-match HEAD 2>/dev/null; then
    VERSION=$(git describe --tags --exact-match HEAD)
else
    VERSION="dev-$(git rev-parse --short HEAD)"
fi

COMMIT=$(git rev-parse HEAD)
DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Check if there are uncommitted changes
if ! git diff-index --quiet HEAD --; then
    print_warning "Building with uncommitted changes"
    VERSION="$VERSION-dirty"
fi

print_info "Building SREootb..."
print_info "Version: $VERSION"
print_info "Commit:  $COMMIT"
print_info "Date:    $DATE"

# Build target list and validate combinations
for os in "${OS_LIST[@]}"; do
    for target_spec in "${TARGET_LIST[@]}"; do
        # Expand "all" if needed
        expanded_targets=$(expand_targets "$os" "$target_spec")
        
        for target in $expanded_targets; do
            # Validate combination
            if validate_combination "$os" "$target"; then
                BUILD_TARGETS+=("${os}-${target}")
            else
                exit 1
            fi
        done
    done
done

print_info "Building targets: ${BUILD_TARGETS[*]}"

# Build flags
LDFLAGS="-X github.com/x86txt/sreootb/cmd.Version=$VERSION -X github.com/x86txt/sreootb/cmd.Commit=$COMMIT -X github.com/x86txt/sreootb/cmd.Date=$DATE"

# Determine if we need dist directory (multiple targets)
if [ ${#BUILD_TARGETS[@]} -eq 1 ]; then
    USE_DIST=false
    OUTPUT_NAME="sreootb"
else
    USE_DIST=true
    DIST_DIR="dist"
    mkdir -p "$DIST_DIR"
fi

# Build each target
for build_target in "${BUILD_TARGETS[@]}"; do
    IFS='-' read -r goos goarch <<< "$build_target"
    
    # Set output filename
    if [ "$USE_DIST" = true ]; then
        if [ "$goos" = "windows" ]; then
            output_file="$DIST_DIR/sreootb-${build_target}.exe"
        else
            output_file="$DIST_DIR/sreootb-${build_target}"
        fi
    else
        if [ "$goos" = "windows" ]; then
            output_file="sreootb.exe"
        else
            output_file="sreootb"
        fi
    fi
    
    print_build "Building $build_target ($goos/$goarch)..."
    
    # Build with cross-compilation
    GOOS="$goos" GOARCH="$goarch" go build \
        -ldflags="$LDFLAGS" \
        -o "$output_file" .
    
    # Check if build was successful
    if [ $? -eq 0 ]; then
        print_info "✅ $build_target build complete: $output_file"
    else
        print_warning "❌ $build_target build failed"
        exit 1
    fi
done

print_info "🎉 All builds complete!"

if [ "$USE_DIST" = true ]; then
    print_info "Binaries available in: $DIST_DIR/"
    echo ""
    print_info "Built targets:"
    ls -la "$DIST_DIR/"
else
    print_info "Test with: ./$output_file --version"
    
    # Test the version output for single builds
    VERSION_OUTPUT=$(./"$output_file" --version)
    print_info "Version output: $VERSION_OUTPUT"
fi 