#!/bin/bash
set -e

# Quick checks
command -v go >/dev/null

# Build go binary - dont change output name, its hard coded in install script
#buildArchitecture="amd64 arm arm64"
buildArchitecture="amd64"
outputEXE="updater"
export CGO_ENABLED=0
export GOARCH=$buildArchitecture
export GOOS=linux
go build -o $outputEXE -a -ldflags '-s -w -buildid= -extldflags "-static"' updater.go

version=$(./$outputEXE -v)
deployerEXE="updater_"$version"_$GOOS-$GOARCH-static"

if [[ $1 == "build" ]]
then
  mv $outputEXE $deployerEXE
  sha256sum $deployerEXE > "$deployerEXE".sha256
fi

exit 0
