# SREootb Makefile
# Organized build system with proper directory structure

.PHONY: build clean run test frontend release release-dry help

# Default target
all: build

# Build the application
build:
	@echo "🔨 Building SREootb..."
	@mkdir -p built
	go build -o built/sreootb .
	@echo "✅ Build complete: built/sreootb"

# Build with version information
build-release:
	@echo "🚀 Building SREootb with version info..."
	@mkdir -p built
	@./scripts/build.sh linux amd64

# Build frontend
frontend:
	@echo "🎨 Building frontend..."
	@cd frontend && npm run build
	@echo "✅ Frontend build complete"

# Clean build artifacts and database files
clean:
	@echo "🧹 Cleaning up..."
	@rm -rf built/
	@rm -rf db/
	@echo "✅ Clean complete"

# Clean only build artifacts (keep database)
clean-build:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf built/
	@echo "✅ Build artifacts cleaned"

# Run the server
run: build
	@echo "🚀 Starting SREootb server..."
	@./built/sreootb standalone --http3 --insecure-tls --bind 0.0.0.0:443

# Run the server in development mode
dev: build
	@echo "🔧 Starting SREootb server in development mode..."
	@./built/sreootb server --config sreootb.example.yaml

# Run tests
test:
	@echo "🧪 Running tests..."
	go test ./...

# Generate configuration files
config:
	@echo "⚙️  Generating configuration files..."
	@./built/sreootb server --gen-config

# Show database status
db-status:
	@echo "📊 Database status:"
	@ls -la db/ 2>/dev/null || echo "No database files found"

# Initialize directories
init:
	@echo "📁 Creating directory structure..."
	@mkdir -p built db certs logs
	@echo "✅ Directory structure created"

# Release management
release-patch:
	@echo "🚀 Creating patch release..."
	@./scripts/release.sh patch

release-minor:
	@echo "🚀 Creating minor release..."
	@./scripts/release.sh minor

release-major:
	@echo "🚀 Creating major release..."
	@./scripts/release.sh major

release:
	@echo "🚀 Creating patch release (default)..."
	@./scripts/release.sh patch

release-dry:
	@echo "🧪 Dry run patch release..."
	@./scripts/release.sh patch --dry-run

# Help target
help:
	@echo "SREootb Build System"
	@echo "==================="
	@echo ""
	@echo "Build Targets:"
	@echo "  build         Build the application (default)"
	@echo "  build-release Build with full version information"
	@echo "  frontend      Build the frontend only"
	@echo "  clean         Clean all build artifacts and database"
	@echo "  clean-build   Clean only build artifacts (keep database)"
	@echo ""
	@echo "Development Targets:"
	@echo "  run           Build and run the server"
	@echo "  dev           Build and run in development mode"
	@echo "  test          Run tests"
	@echo "  config        Generate configuration files"
	@echo ""
	@echo "Release Targets:"
	@echo "  release       Create patch release (default)"
	@echo "  release-patch Create patch release (v1.0.1)"
	@echo "  release-minor Create minor release (v1.1.0)"
	@echo "  release-major Create major release (v2.0.0)"
	@echo "  release-dry   Test release process without changes"
	@echo ""
	@echo "Utility Targets:"
	@echo "  db-status     Show database file status"
	@echo "  init          Create directory structure"
	@echo "  help          Show this help message"
	@echo ""
	@echo "Directory Structure:"
	@echo "  built/        Compiled binaries"
	@echo "  db/           Database files"
	@echo "  certs/        SSL certificates"
	@echo "  logs/         Log files"
	@echo "  frontend/     Frontend source code"
	@echo "  web/          Built frontend assets"
	@echo ""
	@echo "Release Requirements:"
	@echo "  - GitHub CLI (gh) installed and authenticated"
	@echo "  - Clean git working directory"
	@echo "  - Push access to GitHub repository" 
