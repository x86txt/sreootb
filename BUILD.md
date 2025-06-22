# SREootb Build Guide

This document explains the organized build system and directory structure for SREootb.

## Directory Structure

```
sreootb/
├── built/          # Compiled binaries (gitignored)
├── db/             # Database files (gitignored)
├── certs/          # SSL certificates (gitignored)
├── logs/           # Log files (gitignored) 
├── frontend/       # Frontend source code
├── web/            # Built frontend assets (gitignored)
├── internal/       # Go internal packages
├── cmd/            # CLI command definitions
├── scripts/        # Build and deployment scripts
└── Makefile        # Build automation
```

## Quick Start

### Using Makefile (Recommended)

```bash
# Build the application
make build

# Build and run
make run

# Clean build artifacts
make clean-build

# See all available targets
make help
```

### Manual Build

```bash
# Create directories
mkdir -p built db

# Build binary
go build -o built/sreootb .

# Run server
./built/sreootb server
```

## Build Targets

### Single Target (Current Platform)
```bash
make build                    # Quick local build
./scripts/build.sh linux amd64   # Manual single target
```

### Multiple Targets (Cross-compilation)
```bash
./scripts/build.sh linux all          # All Linux architectures
./scripts/build.sh linux,darwin amd64 # Linux and macOS x86_64
make build-release                     # Release build with version info
```

## Directory Organization Benefits

### **`built/`** - Compiled Binaries
- ✅ All binaries in one place
- ✅ Gitignored by default
- ✅ Easy to clean up
- ✅ Supports multiple architecture builds

### **`db/`** - Database Files  
- ✅ Separates data from code
- ✅ Gitignored for security
- ✅ Easy backup/restore
- ✅ Supports multiple database files

### **`.gitignore`** Organization
- ✅ Comprehensive coverage
- ✅ Well-documented sections
- ✅ Security-focused (no secrets in git)
- ✅ Development-friendly

## Configuration

The default paths have been updated:

```yaml
# Old (not organized)
db_path: "./sreootb.db"

# New (organized) 
db_path: "./db/sreootb.db"
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `build` | Build the application (default) |
| `build-release` | Build with full version information |
| `frontend` | Build the frontend only |
| `clean` | Clean all build artifacts and database |
| `clean-build` | Clean only build artifacts (keep database) |
| `run` | Build and run the server |
| `dev` | Build and run in development mode |
| `test` | Run tests |
| `config` | Generate configuration files |
| `db-status` | Show database file status |
| `init` | Create directory structure |
| `help` | Show help message |

## Migration from Old Structure

If you have files in the old locations:

```bash
# Move binaries
mkdir -p built
mv sreootb test-remote-ip built/

# Move database files  
mkdir -p db
mv *.db* db/

# Use new paths
./built/sreootb server
```

## Development Workflow

1. **Clone and setup**:
   ```bash
   git clone <repo>
   cd sreootb
   make init      # Create directories
   ```

2. **Build and test**:
   ```bash
   make build     # Compile
   make test      # Run tests
   make run       # Start server
   ```

3. **Clean builds**:
   ```bash
   make clean-build  # Remove only binaries
   make clean        # Remove binaries and database
   ```

## Release Process

```bash
# Build with version info
make build-release

# Or use the release script
./scripts/release.sh patch
```

This organized structure makes the project cleaner, more maintainable, and follows Go best practices. 