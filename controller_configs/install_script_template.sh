#!/bin/bash
# Ensure script is run in bash
if [ -z "$BASH_VERSION" ]
then
	echo "This script must be run in Bash."
fi

#### Error handling

logError() {
	local errorMessage=$1
	local exitRequested=$2

	echo "[-] Error: $errorMessage"
	if $exitRequested == "true"
	then
		exit 1
	fi
}

#### Pre Checks

# Check for commands
command -v git >/dev/null || logError "git command not found." "true"
command -v egrep >/dev/null || logError "egrep command not found." "true"
command -v sed >/dev/null || logError "sed command not found." "true"
command -v dirname >/dev/null || logError "dirname command not found." "true"
command -v mkdir >/dev/null || logError "mkdir command not found." "true"
command -v echo >/dev/null || logError "echo command not found." "true"
command -v mv >/dev/null || logError "mv command not found." "true"
command -v rm >/dev/null || logError "rm command not found." "true"
command -v cat >/dev/null || logError "cat command not found." "true"
command -v chmod >/dev/null || logError "chmod command not found." "true"
command -v tail >/dev/null || logError "tail command not found." "true"
command -v ls >/dev/null || logError "ls command not found." "true"
command -v tr >/dev/null || logError "tr command not found." "true"

#### Installation
echo "========================================"
echo "     SCMPusher Controller Installer     "
echo "========================================"
read -p "Press enter to begin the installation"
echo "========================================"

#{{DEFAULTS_PLACEHOLDER}}

#### User Choices
echo -e "Provide your choices for the installation. Press enter for the default.\n"

# Exec Path
echo "[*] Enter the full path and file name where you would like the controller executable to be"
read -e -p "    (Default '$executablePath'): " UserChoice_executablePath
if [[ $UserChoice_executablePath != "" ]]
then
	executablePath=$UserChoice_executablePath
fi

# Repo Path
echo "[*] Enter the path in which you would like a new repository to be created"
read -e -p "    (Default '$RepositoryPath'): " UserChoice_RepositoryPath
if [[ $UserChoice_RepositoryPath != "" ]]
then
	RepositoryPath=$UserChoice_RepositoryPath
	# Override default config with new parent dir
	configFilePath="$RepositoryPath/scmpc.yaml"
fi

# Config Path
echo "[*] Enter the full path and file name where you would like the controller config to be"
read -e -p "    (Default '$configFilePath'): " UserChoice_configFilePath
if [[ $UserChoice_configFilePath != "" ]]
then
	configFilePath=$UserChoice_configFilePath
fi

echo "[*] Enter the name of the initial branch for the repository"
read -e -p "    (Default '$BranchName'): " UserChoice_BranchName
if [[ $UserChoice_BranchName != "" ]]
then
	BranchName=$UserChoice_BranchName
fi

echo "[*] Do you want to install the apparmor profile?"
read -e -p " [y/N]: " installAAProfileConfirmation
installAAProfileConfirmation=$(echo $installAAProfileConfirmation | tr [:upper:] [:lower:])
if [[ $installAAProfileConfirmation == "y" ]]
then
	command -v apparmor_parser >/dev/null || logError "apparmor_parser command not found, please install and retry." "true"
fi

# Ask for confirmation before continuing
echo "[*] Are the answers above all correct? Enter 'n' or nothing to exit"
read -e -p "    [y/N]: " ChoicesConfirmation
if [[ $ChoicesConfirmation != "y" ]]
then
	logError "aborting installation" "true"
fi

#### Actions on choices

# Put executable from local dir in user choosen location
PAYLOAD_LINE=$(awk '/^__PAYLOAD_BEGINS__/ { print NR + 1; exit 0; }' $0)
executableDirs=$(dirname $executablePath 2>/dev/null || logError "failed to determine executable parent directories" "true")
mkdir -p $executableDirs 2>/dev/null || logError "failed to create executable parent directory" "true"
tail -n +${PAYLOAD_LINE} $0 | base64 -d | tar -zpvx -C $executableDirs || logError "failed to extract embedded executable" "true"
mv $executableDirs/controller $executablePath 2>/dev/null || logError "failed to move executable" "true"
echo "[+] Successfully extracted deployer binary to $executablePath"

# Run controller to create new repository
$executablePath -n $RepositoryPath:$BranchName || logError "" "true"
cd $RepositoryPath
echo "[+] Successfully created git repository in '$RepositoryPath'"

# create universal dir
mkdir -p $RepositoryPath/$UniversalDirectory 2>/dev/null || logError "failed to create universal directory" "true"
echo "[+] Successfully created Universal Directory at '$RepositoryPath/$UniversalDirectory'"

# Put config in user choosen location
if [[ -f $configFilePath ]]
then
	echo "[-] SSH Config file already exists, not overwritting it. Please configure manually."
else
	cat > "$configFilePath" <<EOF
#{{CONFIG_PLACEHOLDER}}
EOF
	echo "[+] Successfully created controller configuration  in '$configFilePath'"
fi

# Create first commit
GIT_AUTHOR_EMAIL=""
GIT_COMMITTER_EMAIL=""
git add . || logError "failed to git add, please fix error, disable hook, git add and commit" "false"
git commit -m 'Added controller configuration and universal directory' --author 'SCMPController <scmpc@localhost>' || logError "failed to git commit, please fix error, disable hook, and re-commit" "false"
echo "[+] Successfully committed controller files to new repository"

if [[ $installAAProfileConfirmation == "y" ]] then
	# Identify apparmor profile path
	ApparmorProfilePath=/etc/apparmor.d/$(echo $executablePath | sed 's|/|.|')
	#
	cat > "$ApparmorProfilePath" <<EOF
#{{AAPROF_PLACEHOLDER}}
EOF
	#
	apparmor_parser -r $ApparmorProfilePath
	#
fi

echo "[+] New git repository created in $RepositoryPath with initial branch $BranchName and universal directory $UniversalDirectory"
echo "[*] Don't forget to configure your SSH config file with your hosts and environment-specific configuration"

exit 0

# Controller Binary Embed #
__PAYLOAD_BEGINS__
