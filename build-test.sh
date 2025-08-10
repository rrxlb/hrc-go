#!/bin/bash

# Build test script to verify compilation
set -e

echo "Testing Go compilation..."

# Test go mod tidy
echo "Running go mod tidy..."
go mod tidy

# Test go build
echo "Running go build..."
go build -o test-binary .

# Clean up
echo "Cleaning up..."
rm -f test-binary

echo "✅ Build test successful!"