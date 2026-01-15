#!/bin/bash

# Quick start script for ADS-B Scope PWA

echo "🚀 Starting ADS-B Scope PWA..."
echo ""

# Check if Python 3 is available
if ! command -v python3 &> /dev/null; then
    echo "❌ Python 3 is not installed"
    exit 1
fi

# Change to web directory
cd "$(dirname "$0")/web" || exit 1

# Start the server
echo "📡 Starting server on http://localhost:8000"
echo "💡 Login with: admin / admin"
echo ""
echo "Press Ctrl+C to stop"
echo ""

python3 serve.py
