#!/bin/bash
set -e

# Quick checks
command -v go >/dev/null

# Build go binary - dont change output name, its hard coded in install script
#buildArchitecture="amd64 arm arm64"
buildArchitecture="amd64"
export CGO_ENABLED=0
export GOARCH=$buildArchitecture
export GOOS=linux
go build -o updater -a -ldflags '-s -w -buildid= -extldflags "-static"' updater.go

if [[ $1 == "build" ]]
then
  mv updater updater_$GOOS-$GOARCH-static
  sha256sum updater_$GOOS-$GOARCH-static > updater_$GOOS-$GOARCH-static.sha256
fi

exit 0
