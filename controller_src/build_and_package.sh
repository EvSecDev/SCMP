#!/bin/bash
set -e

# Quick checks
command -v go >/dev/null
command -v tar >/dev/null
command -v base64 >/dev/null
command -v sha256sum >/dev/null

# Vars
buildArchitecture="amd64"
export CGO_ENABLED=0
export GOARCH=$buildArchitecture
export GOOS=linux

# Build go binary - dont change output name, its hard coded in install script
go build -o controller -a -ldflags '-s -w -buildid= -extldflags "-static"' controller.go

# Exit if just building binary
if [[ $1 == "build" ]]
then
	exit 0
fi

# Create packaged install script
tar -cvzf controller.tar.gz controller
cp install_controller.sh controller_package_$GOOS-$GOARCH.sh
cat controller.tar.gz | base64 >> controller_package_$GOOS-$GOARCH.sh
rm controller.tar.gz
rm controller
sha256sum controller_package_$GOOS-$GOARCH.sh > controller_package_$GOOS-$GOARCH.sh.sha256

exit 0
