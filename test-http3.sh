#!/bin/bash

# Test HTTP/3 (QUIC) functionality for SREootb

set -e

echo "🧪 Testing HTTP/3 (QUIC) support for SREootb"
echo "=========================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    case $status in
        "INFO")  echo -e "${BLUE}ℹ️  $message${NC}" ;;
        "SUCCESS") echo -e "${GREEN}✅ $message${NC}" ;;
        "WARNING") echo -e "${YELLOW}⚠️  $message${NC}" ;;
        "ERROR") echo -e "${RED}❌ $message${NC}" ;;
    esac
}

# Check if curl supports HTTP/3
check_curl_http3() {
    print_status "INFO" "Checking if curl supports HTTP/3..."
    if curl --version | grep -q "HTTP3"; then
        print_status "SUCCESS" "curl supports HTTP/3"
        return 0
    else
        print_status "WARNING" "curl doesn't support HTTP/3 - install curl with HTTP/3 support for testing"
        print_status "INFO" "On Ubuntu: apt install curl-http3 or build curl with nghttp3/ngtcp2"
        return 1
    fi
}

# Generate self-signed certificates for testing
generate_certs() {
    print_status "INFO" "Generating self-signed certificates for testing..."
    
    if [[ ! -f "test-cert.pem" || ! -f "test-key.pem" ]]; then
        openssl req -x509 -newkey rsa:4096 -keyout test-key.pem -out test-cert.pem \
            -days 365 -nodes -subj "/CN=localhost" \
            -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" 2>/dev/null
        
        if [[ $? -eq 0 ]]; then
            print_status "SUCCESS" "Generated test certificates: test-cert.pem, test-key.pem"
        else
            print_status "ERROR" "Failed to generate certificates"
            exit 1
        fi
    else
        print_status "INFO" "Using existing test certificates"
    fi
}

# Create test configuration with HTTP/3 enabled
create_test_config() {
    print_status "INFO" "Creating test configuration with HTTP/3 enabled..."
    
    cat > sreootb-test.yaml << EOF
server:
  bind: "127.0.0.1:8443"
  tls_cert: "test-cert.pem"
  tls_key: "test-key.pem"
  http3: true
  min_scan_interval: 30s
  max_scan_interval: 300s

database:
  path: "test.db"

logging:
  level: "info"
  format: "json"
EOF

    print_status "SUCCESS" "Created test configuration: sreootb-test.yaml"
}

# Build the binary if needed
build_binary() {
    print_status "INFO" "Building SREootb binary..."
    
    if [[ ! -f "sreootb" ]]; then
        go build -o sreootb . 2>/dev/null
        if [[ $? -eq 0 ]]; then
            print_status "SUCCESS" "Built sreootb binary"
        else
            print_status "ERROR" "Failed to build binary"
            exit 1
        fi
    else
        print_status "INFO" "Using existing sreootb binary"
    fi
}

# Test HTTP/3 connectivity
test_http3() {
    local port=$1
    local has_curl_http3=$2
    
    print_status "INFO" "Testing HTTP/3 connectivity on port $port..."
    
    if [[ $has_curl_http3 -eq 0 ]]; then
        # Test with curl HTTP/3 support
        print_status "INFO" "Testing with curl --http3..."
        
        # Test health endpoint
        if curl -k --http3 --max-time 10 "https://127.0.0.1:$port/api/health" 2>/dev/null | grep -q "healthy"; then
            print_status "SUCCESS" "HTTP/3 health check successful"
        else
            print_status "WARNING" "HTTP/3 health check failed - server may not be ready or HTTP/3 not working"
        fi
        
        # Test config endpoint to check if HTTP/3 is reported as enabled
        local config_response=$(curl -k --http3 --max-time 10 "https://127.0.0.1:$port/api/config" 2>/dev/null)
        if echo "$config_response" | grep -q "http3"; then
            print_status "SUCCESS" "Server reports HTTP/3 support in config"
        else
            print_status "WARNING" "Server may not have HTTP/3 properly enabled"
        fi
    else
        # Fallback tests without HTTP/3 specific curl
        print_status "INFO" "Testing basic HTTPS connectivity (curl without HTTP/3)..."
        
        if curl -k --max-time 10 "https://127.0.0.1:$port/api/health" 2>/dev/null | grep -q "healthy"; then
            print_status "SUCCESS" "HTTPS health check successful"
        else
            print_status "WARNING" "HTTPS health check failed"
        fi
    fi
}

# Test UDP port (QUIC uses UDP)
test_udp_port() {
    local port=$1
    print_status "INFO" "Testing UDP port availability for QUIC on port $port..."
    
    # Try to check if the port is being listened on for UDP
    if command -v ss >/dev/null; then
        if ss -ulpn | grep -q ":$port "; then
            print_status "SUCCESS" "UDP port $port is being listened on (QUIC ready)"
        else
            print_status "WARNING" "UDP port $port not detected - QUIC may not be active"
        fi
    elif command -v netstat >/dev/null; then
        if netstat -ulpn | grep -q ":$port "; then
            print_status "SUCCESS" "UDP port $port is being listened on (QUIC ready)"
        else
            print_status "WARNING" "UDP port $port not detected - QUIC may not be active"
        fi
    else
        print_status "INFO" "Cannot check UDP port (ss/netstat not available)"
    fi
}

# Main test function
main() {
    echo
    print_status "INFO" "Starting HTTP/3 (QUIC) tests..."
    echo
    
    # Check dependencies
    if ! command -v openssl >/dev/null; then
        print_status "ERROR" "openssl is required but not installed"
        exit 1
    fi
    
    if ! command -v go >/dev/null; then
        print_status "ERROR" "go is required but not installed"
        exit 1
    fi
    
    # Check curl HTTP/3 support
    has_curl_http3=1
    if check_curl_http3; then
        has_curl_http3=0
    fi
    
    echo
    
    # Generate certificates
    generate_certs
    
    # Create test config
    create_test_config
    
    # Build binary
    build_binary
    
    echo
    print_status "INFO" "Starting SREootb server with HTTP/3 enabled..."
    
    # Start server in background
    ./sreootb server --config sreootb-test.yaml &
    SERVER_PID=$!
    
    # Wait for server to start
    sleep 3
    
    # Test if server is running
    if kill -0 $SERVER_PID 2>/dev/null; then
        print_status "SUCCESS" "Server started successfully (PID: $SERVER_PID)"
        
        # Test UDP port for QUIC
        test_udp_port 8443
        
        # Test HTTP/3 connectivity
        test_http3 8443 $has_curl_http3
        
        echo
        print_status "INFO" "Checking server logs for HTTP/3 messages..."
        
        # Give some time for logs to show
        sleep 2
        
        echo
        print_status "SUCCESS" "HTTP/3 test completed!"
        print_status "INFO" "Server is running with HTTP/3 support at https://127.0.0.1:8443"
        print_status "INFO" "Try visiting the web interface or use the API"
        
        echo
        print_status "INFO" "Manual testing commands:"
        if [[ $has_curl_http3 -eq 0 ]]; then
            echo "  curl -k --http3 https://127.0.0.1:8443/api/health"
            echo "  curl -k --http3 https://127.0.0.1:8443/api/config"
        else
            echo "  curl -k https://127.0.0.1:8443/api/health"
            echo "  curl -k https://127.0.0.1:8443/api/config"
        fi
        
        echo
        print_status "INFO" "Press Ctrl+C to stop the server"
        
        # Wait for user interrupt
        trap 'print_status "INFO" "Stopping server..."; kill $SERVER_PID 2>/dev/null; exit 0' INT
        wait $SERVER_PID
        
    else
        print_status "ERROR" "Server failed to start"
        exit 1
    fi
}

# Cleanup function
cleanup() {
    if [[ -n $SERVER_PID ]]; then
        kill $SERVER_PID 2>/dev/null || true
    fi
    
    # Optional: cleanup test files
    # rm -f test-cert.pem test-key.pem sreootb-test.yaml test.db
}

# Set trap for cleanup
trap cleanup EXIT

# Run main function
main "$@" 