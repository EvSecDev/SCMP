#!/bin/bash
if [ -z "$BASH_VERSION" ]
then
	echo "This script must be run in BASH."
	exit 1
fi

# Bail on any failure
set -e

# Check for required commands
command -v go >/dev/null
command -v tar >/dev/null
command -v base64 >/dev/null
command -v sha256sum >/dev/null

# Vars
repoRoot=$(pwd)
controllerSRCdir="controller_src"
deployerSRCdir="deployer_src"
updaterSRCdir="deployer_src/updater_src"
signerSRCdir="deployer_src/signing_src"

function usage {
	echo "Usage $0

Options:
  -a <arch>   Architecture of compiled binary (amd64, arm64) [default: amd64]
  -b <prog>   Which program to build (controller, controllerpkg, deployer, deployerpkg, updater)
  -o <os>     Which operating system to build for (linux, windows) [default: linux]
  -f          Build nicely named binary
  -n          Don't add signatures to binary
"
}

function controller_binary {
	# Move into dir
	cd $controllerSRCdir

	# Vars for build
	inputGoSource="*.go"
	outputEXE="controller"
	export CGO_ENABLED=0
	export GOARCH=$1
	export GOOS=$2

	# Build binary
	go build -o $repoRoot/$outputEXE -a -ldflags '-s -w -buildid= -extldflags "-static"' $inputGoSource
	cd $repoRoot

	# Rename to more descriptive if full build was requested
	if [[ $3 == true ]]
	then
		# Get version
		version=$(./$outputEXE -v)
		controllerEXE=""$outputEXE"_"$version"_$GOOS-$GOARCH-static"

		# Rename with version
		mv $outputEXE $controllerEXE
		sha256sum $controllerEXE > "$controllerEXE".sha256
	fi
}

function controller_package {
	# Move into dir
	cd $repoRoot/$controllerSRCdir

	# Vars for build
	inputGoSource="*.go"
	outputEXE="controller"
	export CGO_ENABLED=0
	export GOARCH=$1
	export GOOS=$2

	# Build binary
	go build -o $repoRoot/$outputEXE -a -ldflags '-s -w -buildid= -extldflags "-static"' $inputGoSource
	cd $repoRoot

	# Get version
	version=$(./$outputEXE -v)
	controllerPKG=""$outputEXE"_installer_"$version"_$GOOS-$GOARCH.sh"

	# Create install script
	tar -cvzf $outputEXE.tar.gz $outputEXE
	cp $repoRoot/$controllerSRCdir/install_controller.sh $controllerPKG
	cat $outputEXE.tar.gz | base64 >> $controllerPKG
	sha256sum $controllerPKG > "$controllerPKG".sha256

	# Cleanup
	rm $outputEXE.tar.gz $outputEXE
}

function sign_binary {
	command -v objcopy >/dev/null

	# Set vars
	signerinputfile="$1"
	code_signing_keyfile="/home/admin/Documents/.code_signing_key.priv"
	export GOARCH=amd64
	export GOOS=linux

	# Build sig program
	cd $repoRoot/$signerSRCDir
	go build -o sig -compiler gccgo sig.go

	# Sign
	./sig -in $signerinputfile -priv $code_signing_keyfile -sign
	rm sig
}

function deployer_binary {
	# Move into dir
	cd $deployerSRCdir

	# Vars for build
	inputGoSource="deployer.go"
	outputEXE="deployer"
	export CGO_ENABLED=0
	export GOARCH=$1
	export GOOS=$2

	# Build binary
	go build -o $repoRoot/$outputEXE -a -ldflags '-s -w -buildid= -extldflags "-static"' $inputGoSource
	cd $repoRoot

	# Sign by default, skip if user requested nosig
	if [[ $4 == false ]]
	then
		# Sign deployer
		sign_binary ""$repoRoot"/"$outputEXE""
		cd $repoRoot
	fi

	# Make nicely named binary if requested
	if [[ $3 == true ]]
	then
		# Get version
		version=$(./$outputEXE -v)
		DeployerEXE="deployer_"$version"_$GOOS-$GOARCH-static"

		# Rename with version
		mv $outputEXE $DeployerEXE
		sha256sum $DeployerEXE > "$DeployerEXE".sha256
	fi
}

function deployer_package {
	# Vars for build
	inputGoSource="deployer.go"
	outputEXE="deployer"
	packagingDir="packaging"
	export CGO_ENABLED=0
	export GOARCH=$1
	export GOOS=$2

	# Build binary
	cd $repoRoot/$deployerSRCdir
	go build -o $outputEXE -a -ldflags '-s -w -buildid= -extldflags "-static"' $inputGoSource

	# Sign by default, skip if user requested nosig
	if [[ $3 == false ]]
	then
		# Sign deployer
		sign_binary ""$repoRoot"/"$deployerSRCdir"/"$outputEXE""
		cd $repoRoot/$deployerSRCdir
	fi

	# Get version
	version=$(./$outputEXE -v)

	# Rename package with version
	DeployerPKG=""$outputEXE"_package_"$version"_$GOOS-$GOARCH.tar.gz"

	# Build updater with not nice names
	updater_binary "$GOARCH" "$GOOS" "false"
	updaterEXE=$(find $repoRoot/ -name updater)
	cd $repoRoot/$deployerSRCdir

	# Re-set deployer exe name after overwrite by updater function
	outputEXE="deployer"

	# Create packaged install script
	# Move relevant files into packaging dir
	mkdir $packagingDir
	cp install_deployer.sh $packagingDir/
	mv $outputEXE $packagingDir/$outputEXE
	mv "$updaterEXE" $packagingDir/

	# Create installation tar
	cd $packagingDir
	tar -cvzf ../$DeployerPKG .
	cd $repoRoot/$deployerSRCdir
	rm -rf $packagingDir

	# Add checksum file
	sha256sum $DeployerPKG > "$DeployerPKG".sha256

	# Move files to repo root dir
	mv $DeployerPKG $repoRoot/
	mv "$DeployerPKG".sha256 $repoRoot/
}

function updater_binary {
	# Move into dir
	cd $repoRoot/$updaterSRCdir

	# Vars for build
	inputGoSource="updater.go"
	outputEXE="updater"
	export CGO_ENABLED=0
	export GOARCH=$1
	export GOOS=$2

	# Build binary
	go build -o $repoRoot/$outputEXE -a -ldflags '-s -w -buildid= -extldflags "-static"' $inputGoSource

	# Move back to repo root dir
	cd $repoRoot

	# Rename to more descriptive if full build was requested
	if [[ $3 == true ]]
	then
		# Get version of built binary
		version=$(./$outputEXE -v)
		updaterEXE=""$outputEXE"_"$version"_$GOOS-$GOARCH-static"

		# Rename with version
		mv $outputEXE $updaterEXE
		sha256sum $updaterEXE > $repoRoot/"$updaterEXE".sha256
	fi
}

## START
# DEFAULT CHOICES
buildfull='false'
nosig='false'
architecture="amd64"
os="linux"

# Argument parsing
while getopts 'a:b:o:fnh' opt
do
	case "$opt" in
	  'a')
	    architecture="$OPTARG"
	    ;;
	  'b')
	    buildopt="$OPTARG"
	    ;;
	  'f')
	    buildfull='true'
	    ;;
	  'o')
	    os="$OPTARG"
	    ;;
	  'n')
	    nosig='true'
	    ;;
	  'h')
	    echo "Unknown Option"
	    usage
	    exit 0
 	    ;;
	esac
done

if [[ $buildopt == controller ]]
then
	controller_binary "$architecture" "$os" "$buildfull"
	echo "Complete: controller binary built"
elif [[ $buildopt == controllerpkg ]]
then
	controller_package "$architecture" "$os"
	echo "Complete: controller package built"
elif [[ $buildopt == deployer ]]
then
	deployer_binary "$architecture" "$os" "$buildfull" "$nosig"
	echo "Complete: deployer binary built"
elif [[ $buildopt == deployerpkg ]]
then
	deployer_package "$architecture" "$os" "$nosig"
	echo "Complete: deployer package built"
elif [[ $buildopt == updater ]]
then
	updater_binary "$architecture" "$os" "$buildfull"
	echo "Complete: updater binary built"
fi

exit 0
