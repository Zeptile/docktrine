#!/bin/bash
# Build script for Docktrine
#
#   ./build.sh          build the web UI and the API server for this machine
#   ./build.sh cli      build CLI binaries for macOS and Linux into dist/
#   ./build.sh image    build the server image for the gateway (linux/amd64)
#
# Set IMAGE to name the image (default: docktrine:latest), e.g.
#   IMAGE=registry.zeptile.software/docktrine:latest ./build.sh image
set -e

cd "$(dirname "$0")"

build_local() {
	echo "Building web UI..."
	(cd web && npm install --silent && npm run build)

	echo "Building API server..."
	go build -o bin/docktrine-api ./cmd/api

	echo "Building CLI..."
	go build -o bin/docktrine ./cmd/cli

	echo "Build complete!"
	echo ""
	echo "To run Docktrine:"
	echo "   ./bin/docktrine-api"
	echo ""
	echo "Web UI will be available at: http://localhost:3000"
	echo "API docs will be available at: http://localhost:3000/api/swagger"
}

build_cli() {
	mkdir -p dist
	# The CLI has no cgo dependencies, so it cross-compiles anywhere
	for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do
		os=${target%/*}
		arch=${target#*/}
		echo "Building CLI for $os/$arch..."
		CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w" \
			-o "dist/docktrine-$os-$arch" ./cmd/cli
	done
	echo "CLI binaries in dist/"
}

build_image() {
	image=${IMAGE:-docktrine:latest}
	echo "Building $image for linux/amd64..."
	docker build --platform linux/amd64 -t "$image" .
	echo "Built $image. Push it with: docker push $image"
}

case "${1:-local}" in
	local) build_local ;;
	cli) build_cli ;;
	image) build_image ;;
	*)
		echo "Usage: $0 [local|cli|image]" >&2
		exit 1
		;;
esac
