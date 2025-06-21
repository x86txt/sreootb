#!/bin/bash

# SREootb v2 API Test Script
# This script demonstrates the API functionality

BASE_URL="${1:-http://localhost:8080}"

echo "🚀 Testing SREootb v2 API at $BASE_URL"
echo

# Test health endpoint
echo "📋 Testing health endpoint..."
curl -s "$BASE_URL/api/health" | jq . || echo "Health check response (raw):"
curl -s "$BASE_URL/api/health" && echo
echo

# Test getting stats (empty initially)
echo "📊 Getting initial stats..."
curl -s "$BASE_URL/api/stats" | jq . || echo "Stats response (raw):"
curl -s "$BASE_URL/api/stats" && echo
echo

# Test getting sites (empty initially)
echo "🌐 Getting sites list..."
curl -s "$BASE_URL/api/sites" | jq . || echo "Sites response (raw):"
curl -s "$BASE_URL/api/sites" && echo
echo

# Add a test site
echo "➕ Adding a test site..."
curl -s -X POST "$BASE_URL/api/sites" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://httpbin.org/status/200",
    "name": "HTTPBin Test",
    "scan_interval": "30s"
  }' | jq . || echo "Add site response (raw):"
curl -s -X POST "$BASE_URL/api/sites" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://httpbin.org/status/200",
    "name": "HTTPBin Test",
    "scan_interval": "30s"
  }' && echo
echo

# Add another test site (ping)
echo "🏓 Adding a ping test..."
curl -s -X POST "$BASE_URL/api/sites" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "ping://8.8.8.8",
    "name": "Google DNS",
    "scan_interval": "60s"
  }' | jq . || echo "Add ping site response (raw):"
curl -s -X POST "$BASE_URL/api/sites" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "ping://8.8.8.8",
    "name": "Google DNS", 
    "scan_interval": "60s"
  }' && echo
echo

# Wait a moment for monitoring to start
echo "⏳ Waiting 5 seconds for monitoring to initialize..."
sleep 5

# Trigger manual check
echo "⚡ Triggering manual check..."
curl -s -X POST "$BASE_URL/api/check/manual" \
  -H "Content-Type: application/json" \
  -d '{}' | jq . || echo "Manual check response (raw):"
curl -s -X POST "$BASE_URL/api/check/manual" \
  -H "Content-Type: application/json" \
  -d '{}' && echo
echo

# Wait for check to complete
echo "⏳ Waiting 3 seconds for checks to complete..."
sleep 3

# Get site statuses
echo "📈 Getting site statuses..."
curl -s "$BASE_URL/api/sites/status" | jq . || echo "Status response (raw):"
curl -s "$BASE_URL/api/sites/status" && echo
echo

# Get updated stats
echo "📊 Getting updated stats..."
curl -s "$BASE_URL/api/stats" | jq . || echo "Updated stats response (raw):"
curl -s "$BASE_URL/api/stats" && echo
echo

# Get configuration
echo "⚙️ Getting server configuration..."
curl -s "$BASE_URL/api/config" | jq . || echo "Config response (raw):"
curl -s "$BASE_URL/api/config" && echo
echo

echo "✅ API test completed!"
echo "🌐 Open your browser to $BASE_URL to see the web interface" 