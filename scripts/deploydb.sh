#!/bin/bash

# CockroachDB version to install
COCKROACH_VERSION="v23.2.4"
COCKROACH_URL="https://binaries.cockroachdb.com/cockroach-${COCKROACH_VERSION}.linux-amd64.tgz"

# Node definitions
NODES=(
  "node1:178.156.182.143:10.28.0.4"
  "node2:178.156.191.126:10.28.0.3"
  "node3:178.156.182.135:10.28.0.2"
)

DATA_DIR="/opt/sqldata"
CERTS_DIR="/opt/cockroach-certs"
COCKROACH_USER="cockroach"
COCKROACH_GROUP="cockroach"
COCKROACH_BIN="/usr/local/bin/cockroach"
CLUSTER_NAME="dev-cluster"
MEM_LIMIT="1GiB"   # Conservative for 2GB RAM
CPU_LIMIT="1"      # 1 core

# Helper to run SSH commands
run_ssh() {
  local host="$1"
  shift
  ssh -o StrictHostKeyChecking=no "$host" "$@"
}

# Helper to copy files via scp
copy_scp() {
  local src="$1"
  local host="$2"
  local dest="$3"
  scp -o StrictHostKeyChecking=no "$src" "$host:$dest"
}

# Generate ECDSA CA, node, and client certificates
certs_generate() {
  echo "---- Generating ECDSA CA and node certificates ----"
  rm -rf certs-tmp
  mkdir -p certs-tmp
  pushd certs-tmp > /dev/null

  # Generate CA key and cert
  openssl ecparam -name prime256v1 -genkey -noout -out ca.key
  openssl req -new -x509 -key ca.key -sha256 -days 3650 -subj "/CN=Cockroach CA" -out ca.crt

  # Generate node and client certs
  for node in "${NODES[@]}"; do
    IFS=":" read -r name pub_ip priv_ip <<< "$node"
    # Node key
    openssl ecparam -name prime256v1 -genkey -noout -out node-${name}.key
    # CSR with SANs for both public and private IPs, and localhost
    cat > node-${name}-csr.conf <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no
[req_distinguished_name]
CN = ${name}
[v3_req]
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${name}
IP.1 = ${pub_ip}
IP.2 = ${priv_ip}
IP.3 = 127.0.0.1
EOF
    openssl req -new -key node-${name}.key -out node-${name}.csr -config node-${name}-csr.conf
    openssl x509 -req -in node-${name}.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out node-${name}.crt -days 3650 -sha256 -extfile node-${name}-csr.conf -extensions v3_req
  done

  # Client cert for root
  openssl ecparam -name prime256v1 -genkey -noout -out client.root.key
  cat > client-root-csr.conf <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no
[req_distinguished_name]
CN = root
[v3_req]
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth
EOF
  openssl req -new -key client.root.key -out client.root.csr -config client-root-csr.conf
  openssl x509 -req -in client.root.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.root.crt -days 3650 -sha256 -extfile client-root-csr.conf -extensions v3_req

  popd > /dev/null
}

# Distribute certs to each node
certs_distribute() {
  echo "---- Distributing certificates to nodes ----"
  for node in "${NODES[@]}"; do
    IFS=":" read -r name pub_ip priv_ip <<< "$node"
    run_ssh "$pub_ip" "sudo mkdir -p $CERTS_DIR && sudo chown $COCKROACH_USER:$COCKROACH_GROUP $CERTS_DIR && sudo chmod 700 $CERTS_DIR"
    copy_scp certs-tmp/ca.crt "$pub_ip" "/tmp/ca.crt"
    copy_scp certs-tmp/node-${name}.crt "$pub_ip" "/tmp/node.crt"
    copy_scp certs-tmp/node-${name}.key "$pub_ip" "/tmp/node.key"
    run_ssh "$pub_ip" "sudo mv /tmp/ca.crt $CERTS_DIR/ && sudo mv /tmp/node.crt $CERTS_DIR/ && sudo mv /tmp/node.key $CERTS_DIR/ && sudo chown $COCKROACH_USER:$COCKROACH_GROUP $CERTS_DIR/* && sudo chmod 600 $CERTS_DIR/*"
  done
}

# Distribute client cert to local machine for SQL access
certs_client_local() {
  mkdir -p ~/.cockroach-certs
  cp certs-tmp/ca.crt ~/.cockroach-certs/ca.crt
  cp certs-tmp/client.root.crt ~/.cockroach-certs/client.root.crt
  cp certs-tmp/client.root.key ~/.cockroach-certs/client.root.key
  chmod 600 ~/.cockroach-certs/*
  echo "Client certificates copied to ~/.cockroach-certs for SQL access."
}

# Install CockroachDB and dependencies on a node
install_node() {
  local host="$1"
  local priv_ip="$2"

  echo "---- Installing CockroachDB on $host ($priv_ip) ----"

  run_ssh "$host" bash -s <<EOF
set -e
# Create cockroach user if not exists
if ! id -u $COCKROACH_USER >/dev/null 2>&1; then
  sudo useradd -r -d $DATA_DIR -s /sbin/nologin $COCKROACH_USER
fi

# Download and install CockroachDB
curl -sSL "$COCKROACH_URL" | tar -xz
sudo cp cockroach-${COCKROACH_VERSION}.linux-amd64/cockroach $COCKROACH_BIN
sudo chown root:root $COCKROACH_BIN
sudo chmod 755 $COCKROACH_BIN
rm -rf cockroach-${COCKROACH_VERSION}.linux-amd64

# Create data dir
sudo mkdir -p $DATA_DIR
sudo chown $COCKROACH_USER:$COCKROACH_GROUP $DATA_DIR
sudo chmod 700 $DATA_DIR

# Create certs dir (already done in certs_distribute, but ensure)
sudo mkdir -p $CERTS_DIR
sudo chown $COCKROACH_USER:$COCKROACH_GROUP $CERTS_DIR
sudo chmod 700 $CERTS_DIR

# Create systemd service
sudo tee /etc/systemd/system/cockroach.service > /dev/null <<SERVICE
[Unit]
Description=CockroachDB node
After=network.target

[Service]
Type=notify
User=$COCKROACH_USER
Group=$COCKROACH_GROUP
ExecStart=$COCKROACH_BIN start \
  --certs-dir=$CERTS_DIR \
  --advertise-addr=$priv_ip \
  --listen-addr=$priv_ip:26257 \
  --http-addr=$priv_ip:8080 \
  --join=10.28.0.4:26257,10.28.0.3:26257,10.28.0.2:26257 \
  --cache=256MiB \
  --max-sql-memory=256MiB \
  --store=$DATA_DIR \
  --background
Restart=always
LimitNOFILE=35000
MemoryMax=$MEM_LIMIT
CPUQuota=${CPU_LIMIT}00%
Environment=COCKROACH_SKIP_ENABLING_DIAGNOSTIC_REPORTING=1

[Install]
WantedBy=multi-user.target
SERVICE

sudo systemctl daemon-reload
sudo systemctl enable cockroach
sudo systemctl stop cockroach || true
sudo rm -rf $DATA_DIR/*
EOF
}

# Start CockroachDB on a node
start_node() {
  local host="$1"
  echo "---- Starting CockroachDB on $host ----"
  run_ssh "$host" "sudo systemctl start cockroach"
}

# Initialize the cluster from node1
init_cluster() {
  local host="$1"
  local priv_ip="$2"
  echo "---- Initializing CockroachDB cluster from $host ($priv_ip) ----"
  run_ssh "$host" "sudo -u $COCKROACH_USER $COCKROACH_BIN init --certs-dir=$CERTS_DIR --host=$priv_ip:26257"
}

# Main deployment
echo "=== CockroachDB 3-node secure deployment ==="

certs_generate
certs_distribute
certs_client_local

for node in "${NODES[@]}"; do
  IFS=":" read -r name pub_ip priv_ip <<< "$node"
  install_node "$pub_ip" "$priv_ip"
done

# Start all nodes
for node in "${NODES[@]}"; do
  IFS=":" read -r name pub_ip priv_ip <<< "$node"
  start_node "$pub_ip"
done

# Wait a bit for nodes to start
echo "Waiting 10 seconds for nodes to start..."
sleep 10

# Initialize cluster from node1
IFS=":" read -r name pub_ip priv_ip <<< "${NODES[0]}"
init_cluster "$pub_ip" "$priv_ip"

echo "=== CockroachDB secure cluster deployed! ==="
echo "Web UI: https://${NODES[0]##*:}:8080 (user: root, client cert required)"
echo "SQL:   cockroach sql --certs-dir=~/.cockroach-certs --host=${NODES[0]##*:}:26257"
echo "Data dir: $DATA_DIR"
echo "Certs dir: $CERTS_DIR"
echo "Resource limits: $MEM_LIMIT RAM, $CPU_LIMIT CPU core per node"
echo "To scale up, edit /etc/systemd/system/cockroach.service and restart the service." 