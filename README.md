# SRE: Out of the Box (SREootb) v2

**SREootb** is a modern, lightweight website monitoring and alerting solution that provides both server and agent functionality in a single Go binary. It features an embedded web interface, supports HTTP/2 and HTTP/3, and is designed for simplicity and reliability.

## 🚀 Features

- **Single Binary**: Server and agent functionality in one executable
- **Embedded Web Interface**: Modern, responsive dashboard with real-time updates
- **Modern Protocols**: HTTP/2 and HTTP/3 support with automatic TLS
- **Multiple Monitor Types**: HTTP/HTTPS and ping monitoring
- **High Performance**: Built in Go with efficient monitoring loops
- **Structured Logging**: JSON and console output via zerolog
- **SQLite Database**: Lightweight, zero-configuration database
- **Agent Support**: Deploy monitoring agents across different networks
- **REST API**: Complete API for automation and integration

## 📦 Installation

### Download Binary
```bash
# Download the latest release (when available)
wget https://github.com/sreoob/sreoob/releases/download/v2.0.0/sreoob-linux-amd64
chmod +x sreoob-linux-amd64
sudo mv sreoob-linux-amd64 /usr/local/bin/sreoob
```

### Build from Source
```bash
git clone https://github.com/sreoob/sreoob.git
cd sreoob/v2
go build -o sreoob
```

## 🎯 Quick Start

### 1. Server Mode
Start the monitoring server with embedded web interface:

```bash
# Basic server (HTTP on :8080)
./sreoob server

# Custom configuration
./sreoob server --bind 0.0.0.0:8080 --db-path ./monitoring.db

# With TLS (enables HTTP/2)
./sreoob server --tls-cert ./cert.pem --tls-key ./key.pem

# With HTTP/3 support
./sreoob server --tls-cert ./cert.pem --tls-key ./key.pem --http3
```

### 2. Agent Mode
Deploy monitoring agents on remote networks:

```bash
# Agent connecting to server
./sreoob agent --server-url https://monitor.example.com --api-key YOUR_API_KEY
```

### 3. Access Web Interface
Open your browser to `http://localhost:8080` to access the monitoring dashboard.

## ⚙️ Configuration

### Configuration File
Create `sreootb.yaml` in your working directory:

```yaml
log:
  level: "info"
  format: "console"

server:
  bind: "0.0.0.0:8080"
  db_path: "./sreoob.db"
  min_scan_interval: "10s"
  max_scan_interval: "24h"

agent:
  server_url: "https://your-server.com"
  api_key: "your-64-character-api-key"
  check_interval: "30s"
```

### Environment Variables
All configuration can be set via environment variables:

```bash
export LOG_LEVEL=info
export SERVER_BIND=0.0.0.0:8080
export SERVER_DB_PATH=./sreoob.db
export AGENT_SERVER_URL=https://monitor.example.com
export AGENT_API_KEY=your-api-key
```

## 🔧 CLI Commands

### Server Commands
```bash
# Start server
./sreoob server [flags]

Flags:
  --bind string        Address to bind server to (default "0.0.0.0:8080")
  --db-path string     SQLite database path (default "./sreoob.db")
  --tls-cert string    Path to TLS certificate file
  --tls-key string     Path to TLS private key file
  --http3              Enable HTTP/3 support (requires TLS)
```

### Agent Commands
```bash
# Start agent
./sreoob agent [flags]

Flags:
  --server-url string     URL of SREootb server
  --api-key string        API key for agent authentication
  --agent-id string       Unique identifier for this agent
  --check-interval duration  Interval between server checks (default 30s)
  --bind string           Address for agent health endpoint (default "127.0.0.1:8081")
```

### Global Flags
```bash
  --config string       Config file (default "./sreootb.yaml")
  --log-level string    Log level (trace, debug, info, warn, error) (default "info")
  --log-format string   Log format (console, json) (default "console")
```

## 🌐 API Reference

### Sites Management
```bash
# Get all sites
GET /api/sites

# Get site statuses
GET /api/sites/status

# Add new site
POST /api/sites
{
  "url": "https://example.com",
  "name": "Example Site",
  "scan_interval": "60s"
}

# Get site history
GET /api/sites/{id}/history?limit=100

# Delete site
DELETE /api/sites/{id}
```

### Monitoring
```bash
# Get monitoring statistics
GET /api/stats

# Trigger manual check
POST /api/check/manual
{
  "site_ids": [1, 2, 3]  // Optional: specific sites, omit for all
}

# Health check
GET /api/health
```

### Agent Management
```bash
# List agents
GET /api/agents

# Register agent
POST /api/agents
{
  "name": "Agent Name",
  "api_key": "64-character-api-key",
  "description": "Optional description"
}

# Delete agent
DELETE /api/agents/{id}
```

## 📊 Monitoring Types

### HTTP/HTTPS Monitoring
Monitor web services with customizable intervals:

```json
{
  "url": "https://api.example.com/health",
  "name": "API Health Check",
  "scan_interval": "30s"
}
```

Supports:
- HTTP and HTTPS protocols
- Response time measurement
- Status code validation
- Custom timeouts and retries

### Ping Monitoring
Monitor network connectivity:

```json
{
  "url": "ping://8.8.8.8",
  "name": "Google DNS",
  "scan_interval": "60s"
}
```

Features:
- ICMP ping support
- Network reachability testing
- Configurable intervals

## 🏗️ Architecture

### Single Binary Design
- **Server Mode**: Web interface, API, monitoring engine, database
- **Agent Mode**: Lightweight monitoring client, health endpoint
- **Shared Components**: Configuration, logging, HTTP clients

### Technology Stack
- **Backend**: Go 1.21+, Chi router, SQLite database
- **Frontend**: Embedded HTML/CSS/JavaScript SPA
- **Protocols**: HTTP/1.1, HTTP/2, HTTP/3 (QUIC)
- **Logging**: Zerolog with structured output
- **Database**: SQLite with WAL mode

## 🔒 Security

### TLS Configuration
```bash
# Generate self-signed certificate for testing
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes

# Start with TLS
./sreoob server --tls-cert cert.pem --tls-key key.pem
```

### API Key Management
```bash
# Generate secure API key (64 characters)
openssl rand -hex 32

# Hash for storage (server handles this automatically)
echo -n "your-api-key" | sha256sum
```

## 📈 Performance

### Resource Usage
- **Memory**: ~10-50MB depending on site count
- **CPU**: Minimal, scales with monitoring frequency
- **Storage**: SQLite database grows with check history
- **Network**: Efficient HTTP clients with connection pooling

### Scaling Guidelines
- **Sites**: Tested with 1000+ sites per instance
- **Intervals**: Minimum 10 seconds, maximum 24 hours
- **Agents**: Unlimited agents per server
- **History**: Automatic cleanup recommended for high-frequency monitoring

## 🛠️ Development

### Building
```bash
# Build for current platform
go build -o sreoob

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o sreoob-linux-amd64

# Build with version info
go build -ldflags "-X main.version=2.0.0" -o sreoob
```

### Testing
```bash
# Run tests
go test ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## 📝 Configuration Examples

### Production Server
```yaml
log:
  level: "info"
  format: "json"

server:
  bind: "0.0.0.0:443"
  db_path: "/var/lib/sreoob/sreoob.db"
  tls_cert: "/etc/ssl/certs/sreoob.crt"
  tls_key: "/etc/ssl/private/sreoob.key"
  http3: true
  min_scan_interval: "30s"
  max_scan_interval: "1h"
```

### Development Setup
```yaml
log:
  level: "debug"
  format: "console"

server:
  bind: "127.0.0.1:8080"
  db_path: "./dev.db"
  dev_mode: true
```

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🔗 Links

- **GitHub**: https://github.com/sreoob/sreoob
- **Documentation**: https://docs.sreoob.com
- **Issues**: https://github.com/sreoob/sreoob/issues
- **Releases**: https://github.com/sreoob/sreoob/releases

---

**SRE: Out of the Box** - Simple, reliable, and efficient website monitoring for modern infrastructure. 