# SRE: Out of the Box (SREootb) v2

**SREootb** is a modern, lightweight website monitoring and alerting solution built as a single Go binary with an embedded web interface. Features distributed agent support, user authentication, and high availability options.

## 🚀 Features

- **Single Binary**: Server and agent functionality in one executable
- **User Authentication**: Email/password registration with optional TOTP 2FA
- **Embedded Web Interface**: Modern React dashboard with real-time updates
- **Distributed Monitoring**: Deploy agents across different networks
- **Multiple Protocols**: HTTP/HTTPS, ping monitoring with HTTP/2 and HTTP/3 support
- **High Availability**: CockroachDB cluster support for production deployments
- **Auto-TLS**: Automatic ed25519 certificate generation
- **REST API**: Complete API for automation and integration
- **Structured Logging**: JSON and console output via zerolog

## 📦 Installation

### Download Binary
```bash
# Download the latest release
wget https://github.com/x86txt/sreootb/releases/latest/download/sreootb-linux-amd64
chmod +x sreootb-linux-amd64
sudo mv sreootb-linux-amd64 /usr/local/bin/sreootb
```

### Build from Source
```bash
git clone https://github.com/x86txt/sreootb.git
cd sreootb
go build -o sreootb
```

## 🎯 Quick Start

### 1. Standalone Mode (Recommended)
Run both server and local agent in a single process:

```bash
# Basic standalone mode
./sreootb standalone

# With custom configuration
./sreootb standalone --bind 0.0.0.0:8080 --db-type sqlite

# With CockroachDB for HA
./sreootb standalone --db-type cockroachdb --db-host localhost --db-database sreootb
```

### 2. Server Only Mode
```bash
# Basic server
./sreootb server

# With custom TLS certificates
./sreootb server --tls-cert ./cert.pem --tls-key ./key.pem
```

### 3. Agent Only Mode
```bash
# Agent connecting to server
./sreootb agent --server-url https://monitor.example.com --api-key YOUR_API_KEY
```

### 4. Access Web Interface
1. Open `http://localhost:8080` in your browser
2. Register a new account with email/password
3. Verify your email (check console output in development)
4. Start adding sites to monitor

**Emergency Access**: The master API key is still available for emergency access if needed.

## ⚙️ Configuration

### Generate Configuration
```bash
# Generate sample config for standalone mode
./sreootb standalone --gen-config

# Generate sample config for server mode
./sreootb server --gen-config

# Generate systemd service file
./sreootb server --gen-systemd
```

### Example Configuration (`sreootb.yaml`)
```yaml
log:
  level: "info"
  format: "console"

server:
  bind: "0.0.0.0:8080"
  agent_bind: "0.0.0.0:8081"
  auto_tls: true
  
  database:
    type: "sqlite"
    sqlite_path: "./sreootb.db"
    
    # For CockroachDB HA deployment
    # type: "cockroachdb"
    # host: "10.28.0.4"
    # port: 26257
    # database: "sreootb"
    # user: "sreootb_user"
    # password: "your_secure_password"
    # ssl_mode: "require"
    # ssl_root_cert: "/opt/cockroach-certs/ca.crt"
    # ssl_cert: "/opt/cockroach-certs/client.sreootb_user.crt"
    # ssl_key: "/opt/cockroach-certs/client.sreootb_user.key"
  
  admin_api_key: "your_generated_admin_key_here"
  min_scan_interval: "10s"
  max_scan_interval: "24h"

agent:
  server_url: "https://your-server.com"
  api_key: "your-64-character-api-key"
  agent_id: "unique-agent-name"
  check_interval: "30s"
  bind: "127.0.0.1:8081"
```

## 🔧 CLI Commands

### Standalone Mode
```bash
./sreootb standalone [flags]

Key Flags:
  --bind string              Web server bind address (default "0.0.0.0:8080")
  --agent-bind string        Agent API bind address (default "127.0.0.1:8081")
  --db-type string          Database type: sqlite or cockroachdb (default "sqlite")
  --db-sqlite-path string   SQLite database path (default "./sreootb.db")
  --auto-tls                Enable automatic TLS certificate generation (default true)
  --check-interval duration Agent check interval (default 30s)
```

### Server Mode
```bash
./sreootb server [flags]

Key Flags:
  --bind string        Web server bind address (default "0.0.0.0:8080")
  --agent-bind string  Agent API bind address (default "0.0.0.0:8081")
  --db-type string     Database type: sqlite or cockroachdb
  --auto-tls          Enable automatic TLS (default true)
  --tls-cert string   Custom TLS certificate file
  --tls-key string    Custom TLS private key file
```

### Agent Mode
```bash
./sreootb agent [flags]

Key Flags:
  --server-url string       URL of SREootb server
  --api-key string         API key for agent authentication
  --agent-id string        Unique identifier for this agent
  --check-interval duration Interval between server checks (default 30s)
  --bind string            Address for agent health endpoint (default "127.0.0.1:8081")
```

### Global Flags
```bash
  --config string       Config file (default searches for sreootb.yaml)
  --log-level string    Log level: trace, debug, info, warn, error (default "info")
  --log-format string   Log format: console or json (default "console")
  --gen-config         Generate sample configuration file
  --gen-systemd        Generate systemd service file
```

## 🌐 API Reference

### Authentication
```bash
# Register new user
POST /api/auth/register
{
  "email": "user@example.com",
  "password": "SecurePass123!",
  "first_name": "John",
  "last_name": "Doe"
}

# Login
POST /api/auth/login
{
  "email": "user@example.com",
  "password": "SecurePass123!",
  "totp_code": "123456"  // Optional for 2FA
}

# Master key login (emergency access)
POST /api/auth/master-login
{
  "api_key": "your-master-api-key"
}

# Verify email
POST /api/auth/verify-email
{
  "token": "verification-token-from-email"
}
```

### Sites Management
```bash
# Get all sites
GET /api/sites

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

### Agents
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
```

## 📊 Monitoring Types

### HTTP/HTTPS Monitoring
```json
{
  "url": "https://api.example.com/health",
  "name": "API Health Check",
  "scan_interval": "30s"
}
```

### Ping Monitoring
```json
{
  "url": "ping://8.8.8.8",
  "name": "Google DNS",
  "scan_interval": "60s"
}
```

## 🏗️ Architecture

### Deployment Modes
1. **Standalone**: Single process with server + local agent (recommended)
2. **Server Only**: Central monitoring server
3. **Distributed**: Server + multiple remote agents
4. **High Availability**: CockroachDB cluster with multiple SREootb instances

### Technology Stack
- **Backend**: Go 1.23+, Chi router
- **Database**: SQLite (default) or CockroachDB (HA)
- **Frontend**: Next.js 15, React 19, TailwindCSS
- **Authentication**: bcrypt passwords, TOTP 2FA, session tokens
- **Protocols**: HTTP/1.1, HTTP/2, HTTP/3 (QUIC)

## 🔒 Security

### User Authentication
- **Email/password** registration with strong password requirements
- **Email verification** required for account activation
- **TOTP 2FA** with QR codes and backup codes
- **Session-based** authentication with secure tokens
- **Master API key** preserved for emergency access

### TLS Configuration
```bash
# Auto-TLS (default) - generates ed25519 certificates automatically
./sreootb server --auto-tls

# Custom certificates
./sreootb server --tls-cert cert.pem --tls-key key.pem
```

## 🏢 High Availability Deployment

### CockroachDB Cluster Setup
```bash
# Deploy 3-node CockroachDB cluster with ECDSA certificates
./scripts/deploydb.sh

# Configure SREootb for HA
./sreootb server --db-type cockroachdb --db-host 10.28.0.4 --db-database sreootb
```

See `COCKROACHDB_HA_SETUP.md` for detailed HA deployment instructions.

## 📈 Performance

### Resource Usage
- **Memory**: ~10-100MB depending on site count and agents
- **CPU**: Minimal, scales with monitoring frequency
- **Storage**: Database grows with check history
- **Network**: Efficient with connection pooling

### Scaling Guidelines
- **Sites**: Tested with 1000+ sites per instance
- **Agents**: Unlimited agents per server
- **Intervals**: 10s minimum, 24h maximum
- **HA**: Use CockroachDB cluster for production scale

## 🛠️ Development

### Building
```bash
# Build for current platform
go build -o sreootb

# Build with frontend (requires Node.js)
cd frontend && npm run build:go && cd ..
go build -o sreootb

# Cross-compile (see scripts/build.sh for all platforms)
./scripts/build.sh linux amd64
```

### Dependencies
- Go 1.23+
- Node.js 18+ (for frontend development)
- Optional: CockroachDB for HA deployments

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

---

**SRE: Out of the Box** - Modern website monitoring with user authentication, distributed agents, and high availability options. 