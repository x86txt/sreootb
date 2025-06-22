# CockroachDB High Availability Setup Guide

This guide will walk you through setting up SREootb with CockroachDB for high availability (HA) deployment across 3 servers.

## Architecture Overview

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Server 1      │    │   Server 2      │    │   Server 3      │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │  SREootb    │ │    │ │  SREootb    │ │    │ │  SREootb    │ │
│ │  Instance   │ │    │ │  Instance   │ │    │ │  Instance   │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
│        │        │    │        │        │    │        │        │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │ CockroachDB │◄┼────┼►│ CockroachDB │◄┼────┼►│ CockroachDB │ │
│ │   Node 1    │ │    │ │   Node 2    │ │    │ │   Node 3    │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Prerequisites

- 3 servers (physical or virtual machines)
- Ubuntu 20.04+ or similar Linux distribution
- Network connectivity between all servers
- Sufficient disk space (recommended: 50GB+ per server)
- Root or sudo access on all servers

## Step 1: Install CockroachDB on All Servers

### Download and Install CockroachDB

```bash
# Download CockroachDB binary
curl https://binaries.cockroachdb.com/cockroach-v23.1.14.linux-amd64.tgz | tar -xz

# Move to /usr/local/bin
sudo cp -i cockroach-v23.1.14.linux-amd64/cockroach /usr/local/bin/
sudo chmod +x /usr/local/bin/cockroach

# Verify installation
cockroach version
```

### Create System User and Directories

```bash
# Create cockroach user
sudo useradd -r -s /bin/false cockroach

# Create directories
sudo mkdir -p /var/lib/cockroach /var/log/cockroach /etc/cockroach
sudo chown cockroach:cockroach /var/lib/cockroach /var/log/cockroach /etc/cockroach
```

## Step 2: Generate Certificates (Production Setup)

### On Server 1 (Certificate Authority)

```bash
# Create CA
mkdir certs my-safe-directory
cockroach cert create-ca --certs-dir=certs --ca-key=my-safe-directory/ca.key

# Create node certificates for all servers
cockroach cert create-node server1.example.com server2.example.com server3.example.com localhost 127.0.0.1 --certs-dir=certs --ca-key=my-safe-directory/ca.key

# Create client certificate
cockroach cert create-client root --certs-dir=certs --ca-key=my-safe-directory/ca.key
cockroach cert create-client sreootb_user --certs-dir=certs --ca-key=my-safe-directory/ca.key
```

### Copy Certificates to All Servers

```bash
# Copy certificates to each server
scp certs/* user@server1.example.com:/etc/cockroach/
scp certs/* user@server2.example.com:/etc/cockroach/
scp certs/* user@server3.example.com:/etc/cockroach/

# Set proper permissions on each server
sudo chown cockroach:cockroach /etc/cockroach/*
sudo chmod 600 /etc/cockroach/*.key
sudo chmod 644 /etc/cockroach/*.crt
```

## Step 3: Start CockroachDB Cluster

### Initialize the Cluster (Server 1)

```bash
# Start the first node
cockroach start \
  --certs-dir=/etc/cockroach \
  --store=path=/var/lib/cockroach \
  --listen-addr=server1.example.com:26257 \
  --http-addr=server1.example.com:8080 \
  --join=server1.example.com:26257,server2.example.com:26257,server3.example.com:26257 \
  --background

# Initialize the cluster
cockroach init --certs-dir=/etc/cockroach --host=server1.example.com:26257
```

### Start Remaining Nodes (Server 2 & 3)

```bash
# Server 2
cockroach start \
  --certs-dir=/etc/cockroach \
  --store=path=/var/lib/cockroach \
  --listen-addr=server2.example.com:26257 \
  --http-addr=server2.example.com:8080 \
  --join=server1.example.com:26257,server2.example.com:26257,server3.example.com:26257 \
  --background

# Server 3
cockroach start \
  --certs-dir=/etc/cockroach \
  --store=path=/var/lib/cockroach \
  --listen-addr=server3.example.com:26257 \
  --http-addr=server3.example.com:8080 \
  --join=server1.example.com:26257,server2.example.com:26257,server3.example.com:26257 \
  --background
```

## Step 4: Create Database and User

```bash
# Connect to the cluster
cockroach sql --certs-dir=/etc/cockroach --host=server1.example.com:26257

# In the SQL shell:
CREATE DATABASE sreootb;
CREATE USER sreootb_user WITH PASSWORD 'your_secure_password_here';
GRANT ALL ON DATABASE sreootb TO sreootb_user;
GRANT ALL ON SCHEMA sreootb.public TO sreootb_user;
\q
```

## Step 5: Create Systemd Services

### CockroachDB Service (`/etc/systemd/system/cockroachdb.service`)

```ini
[Unit]
Description=CockroachDB database server
Documentation=https://www.cockroachlabs.com/
After=network.target

[Service]
Type=notify
User=cockroach
ExecStart=/usr/local/bin/cockroach start \
  --certs-dir=/etc/cockroach \
  --store=path=/var/lib/cockroach \
  --listen-addr=SERVER_HOSTNAME:26257 \
  --http-addr=SERVER_HOSTNAME:8080 \
  --join=server1.example.com:26257,server2.example.com:26257,server3.example.com:26257
TimeoutStopSec=60
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=cockroach

[Install]
WantedBy=multi-user.target
```

Replace `SERVER_HOSTNAME` with the appropriate hostname for each server.

## Step 6: Install and Configure SREootb

### Install SREootb Binary

```bash
# Download the latest release
wget https://github.com/x86txt/sreootb/releases/latest/download/sreootb-linux-amd64
sudo mv sreootb-linux-amd64 /usr/local/bin/sreootb
sudo chmod +x /usr/local/bin/sreootb
```

### Generate Configuration

```bash
# Generate server configuration
sreootb server --gen-config
```

### Configure for CockroachDB

Edit `sreootb-server.yaml`:

```yaml
# SRE: Out of the Box (SREootb) Server Configuration
log:
  level: "info"
  format: "console"

server:
  bind: "0.0.0.0:8080"
  agent_bind: "0.0.0.0:8081"
  auto_tls: true
  
  # Database Configuration for CockroachDB
  database:
    type: "cockroachdb"
    host: "localhost"  # Connect to local CockroachDB node for best performance
    port: 26257
    database: "sreootb"
    user: "sreootb_user"
    password: "your_secure_password_here"
    ssl_mode: "require"
    ssl_root_cert: "/etc/cockroach/ca.crt"
    ssl_cert: "/etc/cockroach/client.sreootb_user.crt"
    ssl_key: "/etc/cockroach/client.sreootb_user.key"
    max_open_conns: 25
    max_idle_conns: 5
    conn_max_lifetime: "300s"
    conn_max_idle_time: "60s"
  
  admin_api_key: "your_generated_admin_key_here"
  min_scan_interval: "10s"
  max_scan_interval: "24h"
  dev_mode: false
```

### Create SREootb Systemd Service

```bash
# Generate systemd service file
sreootb server --gen-systemd

# Copy configuration and service files
sudo mkdir -p /etc/sreootb /var/lib/sreootb /var/log/sreootb
sudo cp sreootb-server.yaml /etc/sreootb/
sudo cp sreootb-server.service /etc/systemd/system/

# Create sreootb user
sudo useradd -r -s /bin/false sreootb
sudo chown -R sreootb:sreootb /var/lib/sreootb /var/log/sreootb
```

## Step 7: Start Services

### On Each Server

```bash
# Enable and start CockroachDB
sudo systemctl daemon-reload
sudo systemctl enable cockroachdb
sudo systemctl start cockroachdb

# Check CockroachDB status
sudo systemctl status cockroachdb

# Enable and start SREootb
sudo systemctl enable sreootb-server
sudo systemctl start sreootb-server

# Check SREootb status
sudo systemctl status sreootb-server
```

## Step 8: Verify High Availability

### Test Database Connectivity

```bash
# Check cluster status from any server
cockroach node status --certs-dir=/etc/cockroach --host=localhost:26257
```

### Test Application Connectivity

```bash
# Check SREootb health endpoint
curl https://server1.example.com:8080/api/health
curl https://server2.example.com:8080/api/health
curl https://server3.example.com:8080/api/health
```

### Test Failover

1. Stop one CockroachDB node: `sudo systemctl stop cockroachdb`
2. Verify the application still works on other servers
3. Restart the node: `sudo systemctl start cockroachdb`
4. Verify it rejoins the cluster

## Load Balancer Configuration

For production deployments, use a load balancer to distribute traffic across all three servers:

### HAProxy Example

```haproxy
global
    daemon

defaults
    mode http
    timeout connect 5000ms
    timeout client 50000ms
    timeout server 50000ms

frontend sreootb_frontend
    bind *:443 ssl crt /etc/ssl/certs/sreootb.pem
    redirect scheme https if !{ ssl_fc }
    default_backend sreootb_servers

backend sreootb_servers
    balance roundrobin
    option httpchk GET /api/health
    server server1 server1.example.com:8080 check ssl verify none
    server server2 server2.example.com:8080 check ssl verify none
    server server3 server3.example.com:8080 check ssl verify none
```

## Monitoring and Maintenance

### CockroachDB Monitoring

- Web UI available at: `https://server1.example.com:8080`, `https://server2.example.com:8080`, etc.
- Monitor cluster health, performance metrics, and replication status

### Log Monitoring

```bash
# Monitor CockroachDB logs
sudo journalctl -u cockroachdb -f

# Monitor SREootb logs
sudo journalctl -u sreootb-server -f
```

### Backup Strategy

```bash
# Create database backup
cockroach sql --certs-dir=/etc/cockroach --host=localhost:26257 \
  --execute="BACKUP DATABASE sreootb TO 'nodelocal://1/backup-$(date +%Y%m%d-%H%M%S)';"
```

## Security Considerations

1. **Network Security**: Use firewall rules to restrict access to ports 26257 and 8080
2. **Certificate Management**: Regularly rotate TLS certificates
3. **Password Security**: Use strong passwords and consider rotating them regularly
4. **Access Control**: Limit database user permissions to minimum required

## Troubleshooting

### Common Issues

1. **Connection Errors**: Check certificate paths and permissions
2. **Split Brain**: Ensure proper network connectivity between nodes
3. **Performance Issues**: Monitor connection pool settings and adjust as needed

### Useful Commands

```bash
# Check cluster status
cockroach node status --certs-dir=/etc/cockroach --host=localhost:26257

# Check database connectivity
cockroach sql --certs-dir=/etc/cockroach --host=localhost:26257 --execute="SELECT version();"

# View application logs
sudo journalctl -u sreootb-server --since "1 hour ago"
```

This setup provides robust high availability with automatic failover, data replication, and horizontal scalability for your SREootb monitoring infrastructure. 