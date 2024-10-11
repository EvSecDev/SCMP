#!/bin/bash

function logError {
	echo "Error: $1"
	exit 1
}

# Quick checks
command -v go >/dev/null || logError "go command not found."
command -v tar >/dev/null || logError "tar command not found."
command -v base64 >/dev/null || logError "base64 command not found."

# Build go binary - dont change output name, its hard coded in install script
#buildArchitecture="amd64 arm arm64"
buildArchitecture="amd64"
export CGO_ENABLED=0
export GOARCH=$buildArchitecture
export GOOS=linux
go build -o deployer -a -ldflags '-s -w -buildid= -extldflags "-static"' deployer.go || logError "failed to compile binary"

# Create packaged install script
cp ../install_deployer.sh . || logError "failed to copy install script to pwd"
tar -cvzf deployer.tar.gz deployer || logError "failed to compress binary"
cat deployer.tar.gz | base64 >> install_deployer.sh
mv install_deployer.sh deployer_package_$GOOS-$GOARCH.sh || logError "failed to rename new install script"
rm deployer.tar.gz || "failed to remove gz tar"
rm deployer || "failed to remove binary"

exit 0
