# SREootb Project Context

## Project Overview

**SRE: Out of the Box (SREootb) v2** is a modern, lightweight website monitoring and alerting solution built as a single Go binary with an embedded web interface. The project provides both server and agent functionality in one executable.

## Key Architecture

### Technology Stack
- **Backend**: Go 1.23+ with Chi router
- **Database**: SQLite (default) or CockroachDB (HA deployments)
- **Frontend**: Next.js 15 (SPA mode), React 19, TailwindCSS, Radix UI
- **Protocols**: HTTP/1.1, HTTP/2, HTTP/3 (QUIC) support
- **Logging**: Zerolog with structured JSON and console output
- **Build**: Single binary with embedded frontend assets

### Core Features
- **Monitoring Types**: HTTP/HTTPS and ping monitoring
- **Agent Support**: Distributed monitoring across different networks
- **Auto-TLS**: Automatic ed25519 certificate generation
- **Real-time Updates**: WebSocket connections for live dashboard updates
- **REST API**: Complete API for automation and integration
- **High Availability**: CockroachDB cluster support for production

## Project Structure

```
sreootb/
├── main.go                 # Application entry point with embedded web assets
├── cmd/                    # CLI commands (server, agent, version)
├── internal/              # Go internal packages
│   ├── models/            # Data models and validation
│   ├── database/          # Database layer
│   ├── server/            # HTTP server and API handlers
│   ├── agent/             # Agent functionality
│   ├── monitor/           # Monitoring engine
│   └── config/            # Configuration management
├── frontend/              # Next.js SPA frontend
│   ├── src/app/          # Next.js app router pages
│   ├── src/components/   # React components
│   └── dist/             # Built frontend assets (embedded in Go)
├── built/                 # Compiled binaries (gitignored)
├── db/                    # Database files (gitignored)
├── certs/                 # TLS certificates (gitignored)
├── scripts/              # Build and deployment scripts
└── web/                  # Alternative frontend build location
```

## Key Models and Data Structures

### Core Entities
- **Site**: Websites/endpoints to monitor with configurable intervals
- **SiteCheck**: Individual monitoring check results
- **Agent**: Distributed monitoring agents with authentication
- **MonitorTask**: Monitoring tasks assigned to agents
- **MonitorResult**: Results submitted by agents

### Agent-Focused Architecture (Primary Development Area)
The agent system is the primary focus of development work:

#### Agent Workflow
1. **Agent Registration**: Agents authenticate with server using API keys
2. **Task Assignment**: Server assigns monitoring tasks to available agents
3. **Monitoring Execution**: Agents perform checks on assigned sites/endpoints
4. **Result Submission**: Agents report results back to server
5. **Health Monitoring**: Server tracks agent connectivity and status

#### Key Agent Files
- `internal/agent/agent.go` - Core agent functionality
- `cmd/agent.go` - Agent CLI commands
- `internal/models/models.go` - Agent data structures (Agent, MonitorTask, MonitorResult)
- Agent API endpoints handle registration, task assignment, result submission

#### Agent Configuration
- Server URL and API key authentication
- Check intervals and health monitoring
- Separate bind address for agent health endpoint
- Support for custom agent IDs and descriptions

### Monitoring Types
- **HTTP/HTTPS**: Web service monitoring with response time and status codes
- **Ping**: ICMP network connectivity testing
- Support for custom protocols via URL schemes (http://, https://, ping://)

## CLI Commands

### Available Modes
- **`sreootb standalone`** - Run server + local agent in single process (recommended for simple deployments)
- **`sreootb server`** - Run monitoring server only
- **`sreootb agent`** - Run monitoring agent only

### Command Features
- All commands support `--gen-config` to generate sample configuration files
- Global flags: `--config`, `--log-level`, `--log-format`
- Standalone mode uses SQLite and auto-generates internal API keys
- Built-in help system with `--help` for each command

### Release Commands
- **`make release`** - Create patch release with GitHub integration
- **`make release-minor/major`** - Create minor/major releases
- **`make release-dry`** - Test release process without changes
- **`./scripts/release.sh v1.2.3`** - Create specific version release
- Automatically builds all platforms and uploads to GitHub Releases

## Configuration

### Server Configuration (`sreootb.yaml`)
- **Web GUI**: Configurable bind address, TLS settings
- **Agent API**: Separate endpoint for agent communication
- **Database**: SQLite or CockroachDB with connection pooling
- **Auto-TLS**: Automatic certificate generation and rotation
- **Monitoring**: Configurable scan intervals (10s to 24h)

### Deployment Modes
1. **Standalone Mode**: Single process running both server and local agent with SQLite (NEW - perfect for simple deployments)
2. **Single Instance**: SQLite database, server-only deployment
3. **High Availability**: 3-node CockroachDB cluster with multiple SREootb instances
4. **Agent Network**: Central server with distributed monitoring agents

## Build System

### Organized Structure
- Uses `built/` directory for compiled binaries
- Frontend builds to `web/` or `frontend/dist/`
- Makefile with comprehensive targets
- Cross-compilation support for multiple platforms
- Embedded assets using Go embed directives

### Build Process
1. Frontend builds as static SPA with Next.js export
2. Go embeds frontend assets at compile time
3. Single binary contains all web assets and backend logic
4. Supports version injection during build

### Release System
- **Enhanced release script** (`scripts/release.sh`) with GitHub integration
- **Multi-platform builds** - Linux (amd64, arm64, arm, riscv64), macOS (arm64), Windows (amd64)
- **GitHub CLI integration** - Creates releases and uploads binaries automatically
- **Makefile targets** - `make release`, `make release-patch/minor/major`, `make release-dry`
- **Dry-run support** - Test releases without making changes
- **Auto-generated release notes** - Includes usage examples and download instructions

## Development Patterns

### Developer Profile
- **IDE**: Cursor AI (VSCode fork) with heavy AI assistance (Claude-4-Sonnet MAX mode)
- **Role**: Sole developer handling all aspects - coding, testing, deployment
- **Experience**: Frontend-focused, learning Go as needed
- **Primary Focus**: Agent functionality (used 90% of the time)
- **AI-First Development**: Relies heavily on AI for Go language guidance and architecture decisions

### User Preferences
- User prefers NOT to auto-build or auto-start applications
- Manual build/start instructions preferred over automatic execution
- Focus on uptime, reliability, and performance from the start
- Ad hoc testing approach (formal testing strategy TBD)

### Code Organization
- Clean separation between frontend and backend
- Embedded filesystem for static assets
- RESTful API design with proper validation
- Structured logging throughout
- Configuration via YAML files and environment variables
- **Agent-centric architecture** - most development work centers around agent functionality

## Security Features
- API key-based authentication for agents
- TLS certificate management (auto-generation or custom)
- Secure password hashing for database users
- Input validation and sanitization
- SSL/TLS for database connections in HA mode

## Monitoring Capabilities
- Configurable check intervals (10 seconds to 24 hours)
- Response time measurement
- Status code validation
- Error message capture
- Historical data storage
- Real-time dashboard updates
- Statistics and metrics

## API Endpoints
- Site management (`/api/sites`)
- Agent management (`/api/agents`)
- Monitoring statistics (`/api/stats`)
- Manual checks (`/api/check/manual`)
- Health checks (`/api/health`)
- WebSocket for real-time updates

## High Availability Setup
- 3-node CockroachDB cluster configuration
- Multiple SREootb instances with shared database
- Load balancer configuration (HAProxy example provided)
- Certificate management for multi-node setup
- Automated failover and data replication

## Documentation Available
- Main README with comprehensive setup instructions
- BUILD.md with organized build system details
- COCKROACHDB_HA_SETUP.md with HA deployment guide
- Frontend README with SPA build process
- Configuration examples for various deployment scenarios

## Go Language Context for Frontend Developer

Since agent functionality is the primary focus and requires Go knowledge:

### Key Go Concepts in SREootb
- **Structs**: Data models (Agent, MonitorTask, MonitorResult) defined in `internal/models/`
- **Channels**: Used for agent communication and task coordination
- **Goroutines**: Concurrent monitoring execution across multiple agents
- **HTTP Handlers**: API endpoints in `internal/server/` for agent communication
- **JSON Tags**: Struct field mapping for API requests/responses
- **Error Handling**: Go's explicit error handling pattern throughout codebase
- **Context**: Used for request timeouts and cancellation in monitoring tasks

### Common Go Patterns in Agent Code
- Database operations with proper error handling
- HTTP client creation with timeouts for monitoring checks
- WebSocket connections for real-time agent communication
- Configuration management via Viper (YAML/env vars)
- Structured logging with Zerolog

### Development Approach
- Lean on AI assistance for Go syntax and patterns
- Focus on understanding data flow between agents and server
- Use existing code patterns as templates for new functionality
- Test changes with actual agent deployments for reliability

This project represents a modern, production-ready monitoring solution with careful attention to deployment flexibility and operational simplicity. 