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

controllerCONFdir="controller_configs"
deployerCONFdir="deployer_configs"

function usage {
	echo "Usage $0

Options:
  -b <prog>   Which program to build (controller, controllerpkg, deployer, deployerpkg, updater, signer)
  -a <arch>   Architecture of compiled binary (amd64, arm64) [default: amd64]
  -o <os>     Which operating system to build for (linux, windows) [default: linux]
  -f          Build nicely named binary (does not apply to package builds)
  -n          Don't add signatures to binary
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

function sign_binary {
	command -v objcopy >/dev/null

	# Set vars
	signerinputfile="$1"
	code_signing_keyfile="/home/admin/Documents/.code_signing_key.priv"
	export GOARCH=amd64
	export GOOS=linux

	# Build sig program
	cd $repoRoot/$signerSRCdir
	go build -o sig sig.go

	# Sign
	./sig -in $signerinputfile -priv $code_signing_keyfile -sign
	rm sig
}

function deployer_binary {
	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# Check for things not supposed to be in a release
	check_for_dev_artifacts "$deployerSRCdir"

	# Move into dir
	cd $deployerSRCdir

	# Run tests
	go test

	# Vars for build
	inputGoSource="*.go"
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
		version=$(./$outputEXE --versionid)
		DeployerEXE="deployer_"$version"_$GOOS-$GOARCH-static"

		# Rename with version
		mv $outputEXE $DeployerEXE
		sha256sum $DeployerEXE > "$DeployerEXE".sha256
	fi
}

function deployer_package {
	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# Check for things not supposed to be in a release
	check_for_dev_artifacts "$deployerSRCdir"

	# Read in config files for install script
	defaultInstallOptions="$deployerCONFdir/default_install_options.txt"
	defaultSystemdService="$deployerCONFdir/scmpd.service"
	defaultConfigYaml="$deployerCONFdir/scmpd.yaml"
	defaultApparmorProfile="$deployerCONFdir/apparmor_profile_template"

	# Create installation script
	cp "$deployerCONFdir/install_script_template.sh" "$repoRoot/install_deployer.sh"
	awk '{if ($0 ~ /#{{DEFAULTS_PLACEHOLDER}}/) {while((getline line < "'$defaultInstallOptions'") > 0) print line} else print $0}' install_deployer.sh > .d && mv .d install_deployer.sh
	awk '{if ($0 ~ /#{{CONFIG_PLACEHOLDER}}/) {while((getline line < "'$defaultConfigYaml'") > 0) print line} else print $0}' install_deployer.sh > .d && mv .d install_deployer.sh
	awk '{if ($0 ~ /#{{AAPROF_PLACEHOLDER}}/) {while((getline line < "'$defaultApparmorProfile'") > 0) print line} else print $0}' install_deployer.sh > .d && mv .d install_deployer.sh
	awk '{if ($0 ~ /#{{SYSTEMD_SERVICE_PLACEHOLDER}}/) {while((getline line < "'$defaultSystemdService'") > 0) print line} else print $0}' install_deployer.sh > .d && mv .d install_deployer.sh
	chmod 750 "$repoRoot/install_deployer.sh"

	# Vars for build
	inputGoSource="*.go"
	outputEXE="deployer"
	packagingDir="packaging"
	export CGO_ENABLED=0
	export GOARCH=$1
	export GOOS=$2

	# Build binary
	cd $repoRoot/$deployerSRCdir
	go test
	go build -o $outputEXE -a -ldflags '-s -w -buildid= -extldflags "-static"' $inputGoSource

	# Sign by default, skip if user requested nosig
	if [[ $3 == false ]]
	then
		# Sign deployer
		sign_binary ""$repoRoot"/"$deployerSRCdir"/"$outputEXE""
		cd $repoRoot/$deployerSRCdir
	fi

	# Get version
	version=$(./$outputEXE --versionid)

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
	mv $repoRoot/install_deployer.sh $packagingDir/
	mv $outputEXE $packagingDir/$outputEXE
	mv $updaterEXE $packagingDir/

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
	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# Check for things not supposed to be in a release
	check_for_dev_artifacts "$updaterSRCdir"

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

        # Sign by default, skip if user requested nosig
        if [[ $4 == false ]]
        then
                # Sign updater
                sign_binary ""$repoRoot"/"$outputEXE""
                cd $repoRoot
        fi

	# Rename to more descriptive if full build was requested
	if [[ $3 == true ]]
	then
		# Get version of built binary
		version=$(./$outputEXE --versionid)
		updaterEXE=""$outputEXE"_"$version"_$GOOS-$GOARCH-static"

		# Rename with version
		mv $outputEXE $updaterEXE
		sha256sum $updaterEXE > $repoRoot/"$updaterEXE".sha256
	fi
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
nosig='false'
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
	  'n')
	    nosig='true'
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
	updater_binary "$architecture" "$os" "$buildfull" "$nosig"
	echo "Complete: updater binary built"
fi

exit 0
