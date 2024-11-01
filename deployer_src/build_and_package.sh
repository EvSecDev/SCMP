#!/bin/bash
set -e

if [[ $1 == "-h" ]] || [[ $1 == "--help" ]] || [[ $1 == "help" ]] || [[ $1 == "?" ]]
then
  echo "Usage $0

Options:
  nosigbuild     Only build unsigned deployer executable
  sigbuild	 Only build signed deployer executable
  dontsign	 Build full package, but don't sign deployer executable

No arguments will build full packaged tar
"
  exit 0
fi

# Quick checks
command -v go >/dev/null
command -v tar >/dev/null
command -v sha256sum >/dev/null

# Vars
deployerSrcDir=$(pwd)
code_signing_keyfile="/home/admin/Documents/.code_signing_key.priv"
packagingDir="packaging"
buildArchitecture="amd64"  # Options: "amd64 arm arm64"
export CGO_ENABLED=0
export GOARCH=$buildArchitecture
export GOOS=linux

# Build go binary - dont change output name, its hard coded in install script
go build -o deployer_$GOOS-$GOARCH-static -a -ldflags '-s -w -buildid= -extldflags "-static"' deployer.go

# Exit if only want unsigned deployer build
if [[ $1 == "nosigbuild" ]]
then
	echo "Unsigned Deployer built"
	exit 0
fi

if ! [[ $1 == "dontsign" ]]
then
	# Build sig
	export GOARCH=amd64
	export GOOS=linux
	go build -o sig -compiler gccgo signing_src/sig.go

	# Sign deployer binary
	./sig -in deployer_$GOOS-$GOARCH-static -priv $code_signing_keyfile -sign
fi

# Exit if only want signed deployer build
if [[ $1 == "sigbuild" ]]
then
	echo "Signed Deployer built"
	sha256sum deployer_$GOOS-$GOARCH-static > deployer_$GOOS-$GOARCH-static.sha256
	rm sig
	exit 0
fi

# Build updater
go build -o updater -a -ldflags '-s -w -buildid= -extldflags "-static"' updater_src/updater.go

# Create packaged install script
# Move relevant files into packaging dir
mkdir $packagingDir
cp install_deployer.sh $packagingDir/
mv deployer_$GOOS-$GOARCH-static $packagingDir/deployer
mv updater $packagingDir/

# Create a packaged installation tar
cd $packagingDir
tar -cvzf ../deployer_installer_$GOOS-$GOARCH.tar.gz .
cd $deployerSrcDir
rm -rf $packagingDir

# Add hash file
sha256sum deployer_installer_$GOOS-$GOARCH.tar.gz > deployer_installer_$GOOS-$GOARCH.tar.gz.sha256

# Cleanup
rm sig 2>/dev/null

exit 0
