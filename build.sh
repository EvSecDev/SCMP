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
SRCdir="src"

function usage {
	echo "Usage $0

Options:
  -b          Build the program using defaults
  -r          Replace binary in path with updated one
  -a <arch>   Architecture of compiled binary (amd64, arm64) [default: amd64]
  -o <os>     Which operating system to build for (linux, windows) [default: linux]
  -f          Build nicely named binary
  -u          Update go packages for program
  -g          Generate releases for github
"
}

# Always update README with help menu from code
function update_readme {
	fileName="README.md"
	helpMenuFromMain=$(cat $SRCdir/main.go | sed -n '/const usage = `/,/`/{/^const usage = `$/d; /^`$/d; p;}')

	# Line number for start of md section
	menuSectionStartLineNumber=$(grep -n "### Controller Help Menu" $fileName | cut -d":" -f1)
	helpMenuDelimiter='```'

	# Line number for start of code block
	helpMenuStartLine=$(awk -v startLine="$menuSectionStartLineNumber" -v delimiter="$helpMenuDelimiter" '
	  NR > startLine && $0 ~ delimiter { print NR; exit }
	' "$fileName")

	# Line number for end of code block
	helpMenuEndLine=$(awk -v startLine="$helpMenuStartLine" -v delimiter="$helpMenuDelimiter" '
          NR > startLine && $0 ~ delimiter { print NR; exit }
        ' "$fileName")

	# Replace existing code block with new one
	awk -v start="$helpMenuStartLine" -v end="$helpMenuEndLine" -v replacement="$helpMenuFromMain" '
	    NR < start { print }                # Print lines before the start range
	    NR == start {                       # Print the start line and replacement text
	        print
	        print replacement
	    }
	    NR > start && NR < end { next }     # Skip lines between start and end
	    NR == end { print }                 # Print the end line
	    NR > end { print }                  # Print lines after the end range
	' $fileName > .t && mv .t $fileName
}

function check_for_dev_artifacts {
	# function args
	srcDir=$1

	# Get head commit hash
	headCommitHash=$(git rev-parse HEAD)

        # Get commit where last release was generated from
        lastReleaseCommitHash=$(cat $repoRoot/.last_release_commit)

	# Retrieve the program version from the last release commit
	lastReleaseVersionNumber=$(git show $lastReleaseCommitHash:$srcDir/main.go 2>/dev/null | grep "progVersion string" | cut -d" " -f5 | sed 's/"//g')

	# Get the current version number
	currentVersionNumber=$(grep "progVersion string" $srcDir/main.go | cut -d" " -f5 | sed 's/"//g')

	# Exit if version number hasn't been upped since last commit
	if [[ $lastReleaseVersionNumber == $currentVersionNumber ]] && ! [[ $headCommitHash == $lastReleaseCommitHash ]] && ! [[ -z $lastReleaseVersionNumber ]]
	then
		echo "  [-] Version number in $srcDir/main.go has not been bumped since last commit, exiting build"
		exit 1
	fi

        # Quick check for any left over debug prints
        if egrep -R "DEBUG" $srcDir/*.go
        then
                echo "  [-] Debug print found in source code. You might want to remove that before release."
        fi

	# Quick staticcheck check - ignoring punctuation in error strings
	cd $SRCdir
	set +e
	staticcheck *.go | egrep -v "error strings should not"
	set -e
	cd $repoRoot/
}

function fix_program_package_list_print {
        searchDir="$repoRoot/$SRCdir"
        mainFile=$(grep -il "func main() {" $searchDir/*.go | egrep -v "testing")

	# Hold cumulative (duplicated) imports from all go source files
	allImports=""

	# Loop all go source files
	for gosrcfile in $(find "$searchDir/" -maxdepth 1 -iname *.go)
	do
	        # Get space delimited single line list of imported package names (no quotes) for this go file
	        allImports+=$(cat $gosrcfile | awk '/import \(/,/\)/' | egrep -v "import \(|\)|^\n$" | sed -e 's/"//g' -e 's/\s//g' | tr '\n' ' ' | sed 's/  / /g')
	done

	# Put space delimited list of all the imports into an array
	IFS=' ' read -r -a pkgarr <<< "$allImports"

	# Create associative array for deduping
	declare -A packages

	# Add each import package to the associative array to delete dups
	for pkg in "${pkgarr[@]}"
	do
	        packages["$pkg"]=1
	done

	# Convert back to regular array
	allPackages=("${!packages[@]}")

	# search line in each program that contains package import list for version print
	packagePrintLine='fmt.Print("Direct Package Imports: '

	# Format package list into go print line
	newPackagePrintLine=$'\t\tfmt.Print("Direct Package Imports: '"${allPackages[@]}"'\\n")'

	# Write new package line into go source file that has main function
	sed -i "/$packagePrintLine/c\\$newPackagePrintLine" $mainFile
}

function controller_binary {
	# function args
	buildFull=$3

	# Move built binary to path location
	replaceDeployedExe=$4

	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# Check for things not supposed to be in a release
	check_for_dev_artifacts "$SRCdir"

	# Check for new packages that were imported but not included in version output
	fix_program_package_list_print

	# Ensure readme has updated code blocks
	update_readme

	# Move into dir
	cd $SRCdir

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
	if [[ $buildFull == true ]]
	then
		# Get version
		version=$(./$outputEXE --versionid)
		controllerEXE=""$outputEXE"_"$version"_$GOOS-$GOARCH-static"

		# Rename with version
		mv $outputEXE $controllerEXE
		sha256sum $controllerEXE > "$controllerEXE".sha256
	elif [[ $replaceDeployedExe == true ]]
	then
		# Replace existing binary with new one
		deployedBinaryPath=$(which $outputEXE)
		mv $outputEXE $deployedBinaryPath
	fi
}

function update_go_packages {
	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# Move into src dir
	cd $SRCdir

	# Run go updates
	echo "==== Updating Controller Go packages ===="
	go get -u all
	go mod verify
	go mod tidy
	echo "==== Updates Finished ===="
}

function generate_github_release {
	# Function args
	architecture=$1
	os=$2

	# Build binary and package - call this script (i dont want to figure out why calling the compile functions doesn't work)
	./build.sh -b -f
	mv controller_v* ~/Downloads/

	# Get commit where last release was generated from
	lastReleaseCommitHash=$(cat $repoRoot/.last_release_commit)
	if [[ -z $lastReleaseCommitHash ]]
	then
		echo "Could not determine when last release was by commit, refusing to continue"
		exit 1
	fi

	# Collect commit messages up until the last release commit (not including the release commit messages
	commitMsgsSinceLastRelease=$(git log --format=%B $lastReleaseCommitHash~0..HEAD)

        echo "=================================================="
	# Return early if HEAD is where last release was generated (no messages to format)
	if [[ -z $commitMsgsSinceLastRelease ]]
	then
		echo "No commits since last release"
		return
	fi

	# Format each commit message line by section
	IFS=$'\n'
	for line in $commitMsgsSinceLastRelease
	do
		# Skip empty lines
		if [[ -z $line ]]
		then
			continue
		fi

		# Parse out release message sections
		if [[ $(echo "$line" | egrep "^Added") ]]
		then
			comment_Added="$comment_Added$(echo $line | sed 's/^[ \t]*Added/\n -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')"
		elif [[ $(echo "$line" | egrep "^Changed") ]]
		then
			comment_Changed="$comment_Changed$(echo $line | sed 's/^[ \t]*Changed/\n -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')"
		elif [[ $(echo "$line" | egrep "^Removed") ]]
		then
			comment_Removed="$comment_Removed$(echo $line | sed 's/^[ \t]*Removed/\n -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')"
		elif [[ $(echo "$line" | egrep "^Fixed") ]]
		then
			comment_Fixed="$comment_Fixed$(echo $line | sed 's/^[ \t]*Fixed/\n -/g' | sed 's/bug where //g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')"
		else
			echo "    WARNING: UNSUPPORTED LINE PREFIX: '$line'"
		fi
	done

	# Section headers
	addedHeader="### :white_check_mark: Added"
	changedHeader="### :arrows_counterclockwise: Changed"
	removedHeader="### :x: Removed"
	fixedHeader="### :hammer: Fixed"
	trailerHeader="### :information_source: Instructions"
	trailerComment=" - Please refer to the README.md file for instructions"

	# Combine release message sections
	combinedMsg=""
	if [[ -n $comment_Added ]]
	then
		combinedMsg="$addedHeader$comment_Added\n"
	fi
	if [[ -n $comment_Changed ]]
	then
		combinedMsg="$combinedMsg\n$changedHeader$comment_Changed\n"
	fi
	if [[ -n $comment_Removed ]]
	then
		combinedMsg="$combinedMsg\n$removedHeader$comment_Removed\n"
	fi
	if [[ -n $comment_Fixed ]]
	then
		combinedMsg="$combinedMsg\n$fixedHeader$comment_Fixed\n"
	fi

	# Add standard trailer
	combinedMsg="$combinedMsg\n$trailerHeader\n$trailerComment"

	echo "=================================================="
	echo "RELEASE MESSAGE - CHECK GRAMMA BEFORE PUBLISHING:"
	echo "=================================================="
	echo -e "$combinedMsg"
	echo "=================================================="
	echo "RELEASE ATTACHMENTS"
	ls -l ~/Downloads/
        echo "=================================================="

	# Save commit that this release was made for to track file
	currentReleaseCommitHash=$(git show HEAD --pretty=format:"%H" --no-patch)
	echo $currentReleaseCommitHash > $repoRoot/.last_release_commit
}

## START
# DEFAULT CHOICES
buildfull='false'
architecture="amd64"
os="linux"
replaceDeployedExe='false'

# Argument parsing
while getopts 'a:o:fbnugrh' opt
do
	case "$opt" in
	  'a')
	    architecture="$OPTARG"
	    ;;
	  'b')
	    buildmode='true'
	    ;;
          'r')
            replaceDeployedExe='true'
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
	  'g')
        generate_github_release "$architecture" "$os"
        exit 0
        ;;
	  'h')
	    usage
	    exit 0
 	    ;;
	  *)
	    usage
	    exit 0
 	    ;;
	esac
done

# Act on program args
if [[ $updatepackages == true ]]
then
	# Using the builtopt cd into the src dir and update packages then exit
	update_go_packages
	exit 0
elif [[ $buildmode == true ]]
then
	controller_binary "$architecture" "$os" "$buildfull" "$replaceDeployedExe"
	echo "Complete: controller binary built"
else
	echo "unknown, bye"
	exit 1
fi

exit 0
