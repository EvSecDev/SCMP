#!/bin/bash

function logError {
	echo "Error: $1"
	exit 1
}

# Quick fix for dev box only
gobin='/usr/local/go/bin/go'

# Quick checks
command -v go >/dev/null || logError "go command not found."
command -v tar >/dev/null || logError "tar command not found."
command -v base64 >/dev/null || logError "base64 command not found."

# Build go binary - dont change output name, its hard coded in install script
buildArchitecture="amd64"
export CGO_ENABLED=0
export GOARCH=$buildArchitecture
export GOOS=linux
$gobin build -o controller -a -ldflags '-s -w -buildid= -extldflags "-static"' controller.go || logError "failed to compile binary"

# Create packaged install script
cp ../install_controller.sh . || logError "failed to copy install script to pwd"
tar -cvzf controller.tar.gz controller || logError "failed to compress binary"
cat controller.tar.gz | base64 >> install_controller.sh
mv install_controller.sh controller_package_$GOOS-$GOARCH.sh || logError "failed to rename new install script"
rm controller.tar.gz || "failed to remove gz tar"
rm controller || "failed to remove binary"

exit 0
