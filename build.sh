#!/bin/bash
if [ -z "$BASH_VERSION" ]
then
	echo "This script must be run in BASH." >&2
	exit 1
fi

# Bail on any failure
set -e

# Check for required commands
command -v go >/dev/null
command -v curl >/dev/null
command -v jq >/dev/null
command -v sha256sum >/dev/null
command -v git >/dev/null

# Check for required external variables
if [[ -z $HOME ]]
then
	echo "Missing HOME variable" >&2
	exit 1
fi

# Global Constants/Variables
repoRoot=$(pwd)
readonly SRCdir="src"
readonly sourceFileGlob='*.go'
readonly outputEXE="controller"
readonly READMEmdFileName="README.md"
readonly packagePrintLine='fmt.Print("Direct Package Imports: ' # Line in src file that prints package list for program version argument
readonly readmeHelpMenuStartDelimiter="### Controller Help Menu"
readonly readmeHelpMenuEndDelimiter='```'
readonly srcHelpMenuStartDelimiter="const usage = "

readonly temporaryReleaseDir="$HOME/Downloads/releasetemp"
readonly githubReleaseNotesFile="$temporaryReleaseDir/release-notes.md"
readonly githubRepoName="SCMP"
readonly githubUser="EvSecDev"

# Define colors - unsupported terminals fail safe
if [ -t 1 ] && { [[ "$TERM" =~ "xterm" ]] || [[ "$COLORTERM" == "truecolor" ]] || tput setaf 1 &>/dev/null; }
then
	readonly RED='\033[31m'
	readonly GREEN='\033[32m'
	readonly YELLOW='\033[33m'
	readonly BLUE='\033[34m'
	readonly RESET='\033[0m'
	readonly BOLD='\033[1m'
fi

##################################
# BUILD HELPERS
##################################

function update_readme {
	local helpMenu menuSectionStartLineNumber helpMenuDelimiter helpMenuStartLine helpMenuEndLine

	echo "[*] Copying program help menu from source file to README..."

	# Extract help menu from source code main.go file
	helpMenu=$(cat $SRCdir/main.go | sed -n '/'"$srcHelpMenuStartDelimiter"'`/,/`/{/^'"$srcHelpMenuStartDelimiter"'`$/d; /^`$/d; p;}' | grep -Ev "const usage")

	# Line number for start of md section
	menuSectionStartLineNumber=$(grep -n "$readmeHelpMenuStartDelimiter" $READMEmdFileName | cut -d":" -f1)
	helpMenuDelimiter=$readmeHelpMenuEndDelimiter

	# Line number for start of code block
	helpMenuStartLine=$(awk -v startLine="$menuSectionStartLineNumber" -v delimiter="$helpMenuDelimiter" '
	  NR > startLine && $0 ~ delimiter { print NR; exit }
	' "$READMEmdFileName")

	# Line number for end of code block
	helpMenuEndLine=$(awk -v startLine="$helpMenuStartLine" -v delimiter="$helpMenuDelimiter" '
          NR > startLine && $0 ~ delimiter { print NR; exit }
        ' "$READMEmdFileName")

	# Replace existing code block with new one
	awk -v start="$helpMenuStartLine" -v end="$helpMenuEndLine" -v replacement="$helpMenu" '
	    NR < start { print }                # Print lines before the start range
	    NR == start {                       # Print the start line and replacement text
	        print
	        print replacement
	    }
	    NR > start && NR < end { next }     # Skip lines between start and end
	    NR == end { print }                 # Print the end line
	    NR > end { print }                  # Print lines after the end range
	' $READMEmdFileName > .t && mv .t $READMEmdFileName

	echo -e "   ${GREEN}[+] DONE${RESET}"
}

function check_for_dev_artifacts {
	local srcDir headCommitHash lastReleaseCommitHash lastReleaseVersionNumber currentVersionNumber
	srcDir=$1

	echo "[*] Checking for development artifacts in source code..."

	# Get head commit hash
	headCommitHash=$(git rev-parse HEAD)

    # Get commit where last release was generated from
    lastReleaseCommitHash=$(cat "$repoRoot"/.last_release_commit)

	# Retrieve the program version from the last release commit
	lastReleaseVersionNumber=$(git show "$lastReleaseCommitHash":"$srcDir"/main.go 2>/dev/null | grep "progVersion string" | cut -d" " -f5 | sed 's/"//g')

	# Get the current version number
	currentVersionNumber=$(grep "progVersion string" "$srcDir"/main.go | cut -d" " -f5 | sed 's/"//g')

	# Exit if version number hasn't been upped since last commit
	if [[ $lastReleaseVersionNumber == $currentVersionNumber ]] && ! [[ $headCommitHash == $lastReleaseCommitHash ]] && [[ -n $lastReleaseVersionNumber ]]
	then
		echo -e "   ${RED}[-] ERROR${RESET}: Version number in $srcDir/main.go has not been bumped since last commit, exiting build"
		exit 1
	fi

    # Quick check for any left over debug prints
    if grep -ER "DEBUG" "$srcDir"/*.go
    then
        echo -e "   ${YELLOW}[?] WARNING${RESET}: Debug print found in source code. You might want to remove that before release."
    fi

	# Quick staticcheck check - ignoring punctuation in error strings
	cd $SRCdir
	set +e
	staticcheck ./*.go | grep -Ev "error strings should not"
	set -e
	cd "$repoRoot"/

	echo -e "   ${GREEN}[+] DONE${RESET}"
}

function fix_program_package_list_print {
	local searchDir mainFile allImports IFS pkg allPackages newPackagePrintLine

	echo "[*] Updating import package list in main source file..."

    searchDir="$repoRoot/$SRCdir"

	# Hold cumulative (duplicated) imports from all go source files
	allImports=""

	while IFS= read -r -d '' gosrcfile
	do
        # Get space delimited single line list of imported package names (no quotes) for this go file
        allImports+=$(awk '/import \(/,/\)/' "$gosrcfile" | grep -Ev "import \(|\)|^\n$" | sed -e 's/"//g' -e 's/\s//g' | tr '\n' ' ' | sed 's/  / /g')
	done < <(find "$searchDir/" -maxdepth 1 -type f -iname "*.go" -print0)

	if [[ -z $allImports ]]
	then
		echo -e "   ${RED}[-] ERROR${RESET}: Package import search returned no results"
		exit 1
	fi

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

	if [[ ${#allPackages[@]} == 0 ]]
	then
		echo -e "   ${RED}[-] ERROR${RESET}: Package import deduplication returned no results"
		exit 1
	fi

	# Format package list into go print line
	newPackagePrintLine=$'\t\t'"${packagePrintLine}${allPackages[*]}"'\\n")'

	# Remove testing package
	newPackagePrintLine=${newPackagePrintLine// testing/}

	# Identify if there are no packages in the output
	if echo "$newPackagePrintLine" | grep -qE "^Direct Package Imports:\s+\\\n$"
	then
		echo -e "   ${RED}[-] ERROR${RESET}: New generated package import list is empty"
		exit 1
	fi

    mainFile=$(grep -il "func main() {" "$searchDir"/*.go | grep -Ev "testing")

	# Write new package line into go source file that has main function
	sed -i "/$packagePrintLine/c\\$newPackagePrintLine" "$mainFile"

	echo -e "   ${GREEN}[+] DONE${RESET}"
}

##################################
# MAIN BUILD
##################################

function controller_binary() {
	local GOARCH GOOS buildFull replaceDeployedExe deployedBinaryPath buildVersion
	GOARCH=$1
	GOOS=$2
	buildFull=$3
	replaceDeployedExe=$4

	# Always ensure we start in the root of the repository
	cd "$repoRoot"/

	# Check for things not supposed to be in a release
	check_for_dev_artifacts "$SRCdir"

	# Check for new packages that were imported but not included in version output
	fix_program_package_list_print

	# Ensure readme has updated code blocks
	update_readme

	# Move into dir
	cd $SRCdir

	# Run tests
	echo "[*] Running all tests..."
	go test
	echo -e "   ${GREEN}[+] DONE${RESET}"

	echo "[*] Compiling program binary..."

	# Vars for build
	export CGO_ENABLED=0
	export GOARCH
	export GOOS

	# Build binary
	go build -o "$repoRoot"/"$outputEXE" -a -ldflags '-s -w -buildid= -extldflags "-static"' $sourceFileGlob
	cd "$repoRoot"

	# Get version
	buildVersion=$(./$outputEXE --versionid)

	# Rename to more descriptive if full build was requested
	if [[ $buildFull == true ]]
	then
		local fullNameEXE

		# Rename with version
		fullNameEXE="${outputEXE}_${buildVersion}_${GOOS}-${GOARCH}-static"
		mv "$outputEXE" "$fullNameEXE"

		# Create hash for built binary
		sha256sum "$fullNameEXE" > "$fullNameEXE".sha256
	elif [[ $replaceDeployedExe == true ]]
	then
		# Replace existing binary with new one
		deployedBinaryPath=$(which $outputEXE)
		if [[ -z $deployedBinaryPath ]]
		then
			echo -e "${RED}[-] ERROR${RESET}: Could not determine path of existing program binary, refusing to continue" >&2
			rm "$outputEXE"
			exit 1
		fi

		mv "$outputEXE" "$deployedBinaryPath"
	fi

	echo -e "   ${GREEN}[+] DONE${RESET}: Built version ${BOLD}${BLUE}$buildVersion${RESET}"
}

##################################
# GITHUB Automation
##################################

function create_release_notes() {
	local lastReleaseCommitHash commitMsgsSinceLastRelease IFS commitMsg currentReleaseCommitHash

	echo "[*] Retrieving all git commit messages since last release..."

	# Get commit where last release was generated from
	lastReleaseCommitHash=$(cat "$repoRoot"/.last_release_commit)
	if [[ -z $lastReleaseCommitHash ]]
	then
		echo -e "${RED}[-] ERROR${RESET}: Could not determine when last release was by commit, refusing to continue" >&2
		exit 1
	fi

	# Collect commit messages up until the last release commit (not including the release commit messages
	commitMsgsSinceLastRelease=$(git log --format=%B "$lastReleaseCommitHash"~0..HEAD)

	if [[ -z $commitMsgsSinceLastRelease ]]
	then
		# Return early if HEAD is where last release was generated (no messages to format)
		echo -e "${RED}[-] ERROR${RESET}: No commits since last release" >&2
		exit 1
	fi

	# Format each commit message line by section
	IFS=$'\n'
	for commitMsg in $commitMsgsSinceLastRelease
	do
		# Skip empty lines
		if [[ -z $commitMsg ]]
		then
			continue
		fi

		# Parse out release message sections
		if echo "$commitMsg" | grep -qE "^[aA]dded"
		then
			comment_Added="$comment_Added$(echo "$commitMsg" | sed 's/^[ \t]*[aA]dded/\n -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')"
		elif echo "$commitMsg" | grep -qE "^[cC]hanged"
		then
			comment_Changed="$comment_Changed$(echo "$commitMsg" | sed 's/^[ \t]*[cC]hanged/\n -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')"
		elif echo "$commitMsg" | grep -qE "^[rR]emoved"
		then
			comment_Removed="$comment_Removed$(echo "$commitMsg" | sed 's/^[ \t]*[rR]emoved/\n -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')"
		elif echo "$commitMsg" | grep -qE "^[fF]ixed"
		then
			comment_Fixed="$comment_Fixed$(echo "$commitMsg" | sed 's/^[ \t]*[fF]ixed/\n -/g' | sed 's/bug where //g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')"
		else
			echo -e "   ${YELLOW}[?] WARNING${RESET}: UNSUPPORTED LINE PREFIX: '$commitMsg'"
		fi
	done

	# Release Notes Section headers
	local addedHeader changedHeader removedHeader fixedHeader trailerHeader trailerComment combinedMsg
	addedHeader="### :white_check_mark: Added"
	changedHeader="### :arrows_counterclockwise: Changed"
	removedHeader="### :x: Removed"
	fixedHeader="### :hammer: Fixed"
	trailerHeader="### :information_source: Instructions"
	trailerComment=" - Please refer to the README.md file for instructions"

	# Combine release notes sections
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

	# Save notes to file
	echo -e "$combinedMsg" > "$githubReleaseNotesFile"

	# Save commit that this release was made for to track file
	currentReleaseCommitHash=$(git show HEAD --pretty=format:"%H" --no-patch)
	echo "$currentReleaseCommitHash" > "$repoRoot"/.last_release_commit

	echo "====================================================================="
	echo "RELEASE MESSAGE in $githubReleaseNotesFile - CHECK BEFORE PUBLISHING:"
	echo "====================================================================="
	echo -e "$combinedMsg"
	echo "====================================================================="
	echo "RELEASE ATTACHMENTS in $temporaryReleaseDir"
	echo "====================================================================="
	find "$temporaryReleaseDir"/ -maxdepth 1 -type f ! -iwholename "$githubReleaseNotesFile"
}

function create_github_release() {
	local versionTag releaseNotes releaseMeta curlOutput releaseID finalReleaseURL
	versionTag=$1

	if [[ -z $GITHUB_API_TOKEN ]]
	then
		echo -e "   ${RED}[-] ERROR${RESET}: GITHUB_API_TOKEN env variable is not set" >&2
		exit 1
	fi

	echo "[*] Creating new Github release with notes from file $githubReleaseNotesFile"

	releaseNotes=$(cat "$githubReleaseNotesFile")
	if [[ -z $releaseNotes ]]
	then
		echo -e "   ${RED}[-] ERROR${RESET}: Unable to read contents of release notes file $githubReleaseNotesFile" >&2
		exit 1
	fi

	# Escape newlines and carriage returns for inclusion in JSON
	releaseNotes=$(echo "$releaseNotes" | sed ':a;N;$!ba;s/\n/\\n/g')

	releaseMeta='{"tag_name":"'$versionTag'","target_commitish":"main","name":"","body":"'$releaseNotes'","draft":false,"prerelease":false,"generate_release_notes":false}'
	if ! jq . <<< "$releaseMeta" >/dev/null
	then
		echo -e "   ${RED}[-] ERROR${RESET}: Invalid release JSON, please check for unsupported characters in release notes" >&2
		exit 1
	fi

	curlOutput=$(curl --silent -L \
  -X POST \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer $GITHUB_API_TOKEN" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  -d "$releaseMeta" \
    'https://api.github.com/repos/'"$githubUser"'/'"$githubRepoName"'/releases')

	if [[ -z $curlOutput ]]
	then
		echo -e "   ${RED}[-] ERROR${RESET}: Received no response from github post to create release" >&2
		exit 1
	fi

	releaseID=$(jq -r .id <<< "$curlOutput")
	if [[ -z $releaseID ]] || [[ $releaseID == null ]]
	then
		errorResponse=$(jq -r .status <<< "$curlOutput")
		errorMessage=$(jq -r .message <<< "$curlOutput") 

		echo -e "   ${RED}[-] ERROR${RESET}: Unable to extract release ID from github response. ($errorResponse) $errorMessage" >&2
		exit 1
	fi

	finalReleaseURL=$(jq -r .url <<< "$curlOutput")

	echo -e "${GREEN}[+] Successfully${RESET} created new Github release - ID: $releaseID"
	rm "$githubReleaseNotesFile"

	cd "$temporaryReleaseDir"

	# Upload every file that is not the notes in the temp dir
	local localFileName
	while IFS= read -r  -d '' localFileName
	do
		echo "  [*] Uploading file $localFileName to release $releaseID"

		curlOutput=$(curl --silent -L \
  	-X POST \
  	-H "Accept: application/vnd.github+json" \
  	-H "Authorization: Bearer $GITHUB_API_TOKEN" \
  	-H "X-GitHub-Api-Version: 2022-11-28" \
  	-H "Content-Type: application/octet-stream" \
  	--data-binary "@$localFileName" \
  	'https://uploads.github.com/repos/'"$githubUser"'/'"$githubRepoName"'/releases/'"$releaseID"'/assets?name='"$localFileName")

		if [[ -z $curlOutput ]]
		then
			echo -e "   ${RED}[-] ERROR${RESET}: Received no response from github post to upload attachments" >&2
			exit 1
		fi

		uploadState=$(jq -r .state <<< "$curlOutput")
		if [[ $uploadState != uploaded ]]  || [[ $uploadState == null ]]
		then
			echo -e "   ${RED}[-] ERROR${RESET}: Expected state to be uploaded but got $uploadState from github" >&2
			exit 1
		fi

		echo -e "  ${GREEN}[+] Successfully${RESET} uploaded file $localFileName to release $releaseID"
	done < <(find . -maxdepth 1 -type f -print0 | sed 's|\./||g')

	echo -e "${GREEN}[+]${RESET} Release published: $finalReleaseURL"

	# Cleanup
	rm -r "$temporaryReleaseDir"
	cd "$repoRoot"/
}

##################################
# Quick Helpers
##################################

function update_go_packages {
	cd "$repoRoot"/
	cd $SRCdir

	echo "[*] Updating Controller Go packages..."
	go get -u all
	if [[ $? != 0 ]]
	then
		echo -e "${RED}[-] ERROR${RESET}: Go module update failed"
		return
	fi

	go mod verify
	if [[ $? != 0 ]]
	then
		echo -e "${RED}[-] ERROR${RESET}: Go module verification failed"
		return
	fi

	go mod tidy
	echo -e "   ${GREEN}[+] DONE${RESET}"
}

##################################
# START
##################################

function usage {
	echo "Usage $0
Program Build Script and Helpers

Options:
  -b           Build the program using defaults
  -r           Replace binary in path with updated one
  -a <arch>    Architecture of compiled binary (amd64, arm64) [default: amd64]
  -o <os>      Which operating system to build for (linux, windows) [default: linux]
  -f           Build nicely named binary
  -u           Update go packages for program
  -p           Prepare release notes and attachments
  -P <version> Publish release to github
  -h           Print this help menu
"
}

# DEFAULT CHOICES
buildfull='false'
architecture="amd64"
os="linux"
replaceDeployedExe='false'
prepareRelease='false'
publishRelease='false'

# Argument parsing
while getopts 'a:o:P:fbnuprh' opt
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
	  'p')
        prepareRelease='true'
        ;;
	  'P')
	    publishRelease='true'
		publishVersion="$OPTARG"
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

if [[ $prepareRelease == true ]]
then
	mkdir -p "$temporaryReleaseDir"
	if [[ $? != 0 ]]
	then
		echo -e "${RED}ERROR${RESET}: Unable to create temp release dir, cannot continue" >&2
		exit 1
	fi

	controller_binary "$architecture" "$os" 'true' 'false'
	mv controller_v* "$temporaryReleaseDir"/
	if [[ $? != 0 ]]
	then
		echo -e "${RED}ERROR${RESET}: Unable to put binaries in temp release dir, cannot continue" >&2
		exit 1
	fi

	create_release_notes
elif [[ $publishRelease == true ]]
then
	create_github_release "$publishVersion"
elif [[ $updatepackages == true ]]
then
	update_go_packages
elif [[ $buildmode == true ]]
then
	controller_binary "$architecture" "$os" "$buildfull" "$replaceDeployedExe"
else
	echo -e "${RED}ERROR${RESET}: Unknown option or combination of options" >&2
	exit 1
fi

exit 0
