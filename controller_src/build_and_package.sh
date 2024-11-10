#!/bin/bash
set -e

# Quick checks
command -v go >/dev/null
command -v tar >/dev/null
command -v base64 >/dev/null
command -v sha256sum >/dev/null

# Vars
buildArchitecture="amd64"
outputEXE="controller"
export CGO_ENABLED=0
export GOARCH=$buildArchitecture
export GOOS=linux

# Build go binary - dont change output name, its hard coded in install script
go build -o $outputEXE -a -ldflags '-s -w -buildid= -extldflags "-static"' *.go

# New names
version=$(./$outputEXE -v)
controllerEXE="controller_"$version"_$GOOS-$GOARCH-static"
controllerPKG="controller_package_"$version"_$GOOS-$GOARCH.sh"

# Exit if just building binary
if [[ $1 == "build" ]]
then
	mv $outputEXE $controllerEXE
	sha256sum $controllerEXE > "$controllerEXE".sha256
	exit 0
fi

# Create packaged install script
tar -cvzf controller.tar.gz $outputEXE
cp install_controller.sh $controllerPKG
cat controller.tar.gz | base64 >> $controllerPKG
sha256sum $controllerPKG > "$controllerPKG".sha256

# Cleanup
rm controller.tar.gz $outputEXE

exit 0
