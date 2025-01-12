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
command -v base64 >/dev/null
command -v sha256sum >/dev/null

# Vars
repoRoot=$(pwd)
controllerSRCdir="controller_src"
controllerCONFdir="controller_configs"

function usage {
	echo "Usage $0

Options:
  -b <prog>   Which program to build (controller, controllerpkg)
  -a <arch>   Architecture of compiled binary (amd64, arm64) [default: amd64]
  -o <os>     Which operating system to build for (linux, windows) [default: linux]
  -f          Build nicely named binary (does not apply to package builds)
  -u          Update go packages for a given program (use -b to choose which program, *pkg options not applicable)
"
}

function check_for_dev_artifacts {
	srcDir=$1
        # Quick check for any left over debug prints
        if egrep -R "DEBUG" $srcDir/*.go
        then
                echo "  Debug print found in source code. You might want to remove that before release."
        fi

	# Quick staticcheck check - ignoring punctuation in error strings
	cd $controllerSRCdir
	set +e
	staticcheck *.go | egrep -v "error strings should not"
	set -e
	cd $repoRoot/
}

function controller_binary {
	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# Check for things not supposed to be in a release
	check_for_dev_artifacts "$controllerSRCdir"

	# Move into dir
	cd $controllerSRCdir

	# Run tests
	go test

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
		version=$(./$outputEXE --versionid)
		controllerEXE=""$outputEXE"_"$version"_$GOOS-$GOARCH-static"

		# Rename with version
		mv $outputEXE $controllerEXE
		sha256sum $controllerEXE > "$controllerEXE".sha256
	fi
}

function controller_package {
	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# Check for things not supposed to be in a release
	check_for_dev_artifacts "$controllerSRCdir"

	# Read in config files for install script
	defaultInstallOptions="$controllerCONFdir/default_install_options.txt"
	defaultConfigYaml="$controllerCONFdir/scmpc.yaml"
	defaultApparmorProfile="$controllerCONFdir/apparmor_profile_template"

	# Create installation script
	cp "$controllerCONFdir/install_script_template.sh" "$repoRoot/install_controller.sh"
	awk '{if ($0 ~ /#{{DEFAULTS_PLACEHOLDER}}/) {while((getline line < "'$defaultInstallOptions'") > 0) print line} else print $0}' install_controller.sh > .d && mv .d install_controller.sh
	awk '{if ($0 ~ /#{{CONFIG_PLACEHOLDER}}/) {while((getline line < "'$defaultConfigYaml'") > 0) print line} else print $0}' install_controller.sh > .d && mv .d install_controller.sh
	awk '{if ($0 ~ /#{{AAPROF_PLACEHOLDER}}/) {while((getline line < "'$defaultApparmorProfile'") > 0) print line} else print $0}' install_controller.sh > .d && mv .d install_controller.sh
	chmod 750 "$repoRoot/install_controller.sh"

	# Move into src dir
	cd $repoRoot/$controllerSRCdir

	# Run tests
	go test

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
	version=$(./$outputEXE --versionid)
	controllerPKG=""$outputEXE"_installer_"$version"_$GOOS-$GOARCH.sh"

	# Create install script with embedded executable
	tar -cvzf $outputEXE.tar.gz $outputEXE
	mv $repoRoot/install_controller.sh $controllerPKG
	cat $outputEXE.tar.gz | base64 >> $controllerPKG
	sha256sum $controllerPKG > "$controllerPKG".sha256

	# Cleanup
	rm $outputEXE.tar.gz $outputEXE
}

function update_go_packages {
	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# By program, id src directory variable name
	srcDirVar="${1}SRCdir"

	# Move into src dir
	cd ${!srcDirVar}

	# Run go updates
	echo "==== Updating $1 Go packages ===="
	go get -u all
	go mod verify
	go mod tidy
	echo "==== Updates Finished ===="
}

## START
# DEFAULT CHOICES
buildfull='false'
architecture="amd64"
os="linux"

# Argument parsing
while getopts 'a:b:o:fnuh' opt
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
	  'u')
	    updatepackages='true'
	    ;;
	  'h')
	    usage
	    exit 0
 	    ;;
	esac
done

# Using the builtopt cd into the src dir and update packages then exit
if [[ $updatepackages == true ]]
then
	update_go_packages "$buildopt"
	exit 0
fi

# Binary builds
if [[ $buildopt == controller ]]
then
	controller_binary "$architecture" "$os" "$buildfull"
	echo "Complete: controller binary built"
elif [[ $buildopt == controllerpkg ]]
then
	controller_package "$architecture" "$os"
	echo "Complete: controller package built"
fi

exit 0
