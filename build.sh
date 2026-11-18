#!/bin/bash

# Build script for Docktrine
set -e

echo "🏗️  Building Docktrine..."

# Build web UI
echo "📦 Building web UI..."
cd web
npm install --silent
npm run build
cd ..

# Build Go API server
echo "🚀 Building API server..."
go build -o bin/docktrine cmd/api/main.go

echo "✅ Build complete!"
echo ""
echo "🌟 To run Docktrine:"
echo "   ./bin/docktrine"
echo ""
echo "🌐 Web UI will be available at: http://localhost:3000"
echo "📚 API docs will be available at: http://localhost:3000/api/swagger" 