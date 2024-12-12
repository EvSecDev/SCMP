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
command -v ssh >/dev/null || logError "ssh command not found." "true"
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
command -v ssh-keygen >/dev/null || logError "ssh-keygen command not found." "true"

#### Installation
echo "========================================"
echo "     SCMPusher Controller Installer     "
echo "========================================"
read -p "Press enter to begin the installation"
echo "========================================"

# Default choices
executablePath="/usr/local/bin/scmcontroller"
RepositoryPath="~/SCMGit"
BranchName="main"
configFilePath="$RepositoryPath/scmpc.yaml"
LogJournalBool="false"
UniversalDirectory="universalconfigs"
SSHIdentityPath="~/.ssh/scmp_ssh.key"
SSHPort="2022"
MaximumOutboundConnections="10"
SudoPassword=""
KnownHostsFile="$RepositoryPath/.known_hosts"
remoteTransferBuffer="/tmp/.scmpbuffer"
remoteBackupDir="/tmp/.scmpbackups"

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

## Config user choices
# ssh key path
echo "[*] Enter the full path and file name to the SSH private key for the controller (ed25519 key type required)"
read -e -p "    (Default '$SSHIdentityPath'): " UserChoice_SSHIdentityPath
if [[ $UserChoice_SSHIdentityPath != "" ]]
then
	SSHIdentityPath=$UserChoice_SSHIdentityPath
	# user supplied only if the key exists
	if [[ $(ls $SSHIdentityPath 2>&1 1>/dev/null) ]]
	then
		UserSuppliedKey="true"
	else
		UserSuppliedKey="false"
	fi
fi

# Max outbound connections at once
echo "[*] Enter the maximum number of allowed outbound connections for the controller"
read -e -p "    (Default '$MaximumOutboundConnections'): " UserChoice_MaximumOutboundConnections
if [[ $UserChoice_MaximumOutboundConnections != "" ]]
then
	MaximumOutboundConnections=$UserChoice_MaximumOutboundConnections
fi

# SSH User password
echo "[*] Enter the SSH password for the user of all remote hosts (this will be used for sudo only, leave blank for unrestricted sudo access)"
read -e -s -p "    Password: " UserChoice_SudoPassword
if [[ $UserChoice_SudoPassword != "" ]]
then
	read -e -s -p "    Password again: " UserChoice_SudoPasswordConfirm
	if [[ $UserChoice_SudoPassword == $UserChoice_SudoPasswordConfirm ]]
	then
		SudoPassword=$UserChoice_SudoPassword
	else
		logError "Passwords do not match, try again." "true"
	fi
fi

echo "[*] Enter the name of the initial branch for the repository"
read -e -p "    (Default '$BranchName'): " UserChoice_BranchName
if [[ $UserChoice_BranchName != "" ]]
then
	BranchName=$UserChoice_BranchName
fi

echo "[*] Enter the name of the universal directory inside the repository"
read -e -p "    (Default '$UniversalDirectory'): " UserChoice_UniversalDirectory
if [[ $UserChoice_UniversalDirectory != "" ]]
then
	UniversalDirectory=$UserChoice_UniversalDirectory
fi

echo "[*] Add all the SSH hosts in your SSH config file to the deployer endpoints?"
read -e -p "    [y\N]: " AddSSHConfigHostsConfirmation
AddSSHConfigHostsConfirmation=$(echo $AddSSHConfigHostsConfirmation | tr [:upper:] [:lower:])

echo "[*] Will you be using the Deployer SSH server instead of the standard SSH server?"
read -e -p "    [y/N]: " RemoteUsesDeployerServer
RemoteUsesDeployerServer=$(echo $RemoteUsesDeployerServer | tr [:upper:] [:lower:])

echo "[*] Install git post-commit hook? (REQUIRED if operating controller in auto-deploy mode)"
read -e -p "    [y\N]: " InstallHookConfirmation
InstallHookConfirmation=$(echo $InstallHookConfirmation | tr [:upper:] [:lower:])

echo "[*] Log controller errors to journald?"
read -e -p "    [y/N]: " LogJournalBoolConfirmation
LogJournalBoolConfirmation=$(echo $LogJournalBoolConfirmation | tr [:upper:] [:lower:])

echo "[*] Do you want to install the apparmor profile?"
read -e -p " [y/N]: " installAAProfileConfirmation
installAAProfileConfirmation=$(echo $installAAProfileConfirmation | tr [:upper:] [:lower:])
if [[ $installAAProfileConfirmation == "y" ]]
then
	command -v apparmor_parser >/dev/null || logError "apparmor_parser command not found, please install and retry." "true"
fi

echo "[*] Use your SSH agent to retrieve private keys?"
read -e -p "    [y/N]: " SSHAgentBoolConfirmation
SSHAgentBoolConfirmation=$(echo $SSHAgentBoolConfirmation | tr [:upper:] [:lower:])

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
echo "[+] Successfully created git repository in '$RepositoryPath'"

# create universal dir
mkdir -p $RepositoryPath/$UniversalDirectory 2>/dev/null || logError "failed to create universal directory" "true"
echo "[+] Successfully created Universal Directory at '$RepositoryPath/$UniversalDirectory'"

# SSH key generation
if [[ $UserSuppliedKey != "true" ]]
then
	ssh-keygen -t ed25519 -N '' -C scmp/controller -f $SSHIdentityPath || logError "failed to generate private key" "true"
	SSHPublicKey=$(cat $SSHIdentityPath.pub)
	rm $SSHIdentityPath.pub
	echo "[+] Successfully created new SSH private key in $SSHIdentityPath"
elif [[ $UserSuppliedKey == "true" ]]
then
	SSHPublicKey=$(ssh-keygen -y -f $SSHIdentityPath) || logError "invalid ssh key in $SSHIdentityPath" "true"
	echo "[+] Validated supplied SSH key at $SSHIdentityPath"
fi

# Journald log choice
if [[ $LogJournalBoolConfirmation == "y" ]]
then
	LogJournalBool="true"
else
	LogJournalBool="false"
fi

# SSH Agent choice
if [[ $SSHAgentBoolConfirmation == "y" ]]
then
	SSHAgentBool="true"
else
	SSHAgentBool="false"
fi

# Put config in user choosen location
cat > "$configFilePath" <<EOF
Controller:
  # Path to the root of the git repository
  RepositoryPath: "$repositoryPath"
  LogtoJournald: $LogJournalBool
SSHClient:
  # Change where remote hosts public keys will be stored (don't use .ssh/known_hosts) - recommended to keep in the root of the repository (otherwise, changed your apparmor profile)
  KnownHostsFile: "$KnownHostsFile"
  # Limit number of ssh outbound connections at once
  MaximumConnectionsAtOnce: $MaximumOutboundConnections
# Defaults for SSH Client - applies to all endpoints. Can be overriden under each endpoint host
SSHClientDefaults:
  endpointPort: 2022
  endpointUser: "deployer"
  # File path for client's SSH key
  SSHIdentityFile: "$SSHIdentityPath"
  # Set to true if you want to use your SSH agent to retrieve the private key (requires pubkey in identity file)
  UseSSHAgent: $SSHAgentBool
  # Password that will be used to run sudo commands on remote host
  # Leave blank if sudo does not require a password
  SudoPassword: "$SudoPassword"
  # Remote file that is used for unprivileged file transfers
  RemoteTransferBuffer: "$remoteTransferBuffer"
  # Remote directory that is temporarily used to backup files in case deployment fails
  RemoteBackupDir: "$remoteBackupDir"
# Repo dir to house all configs that should be deployed to every host
UniversalDirectory: "$UniversalDirectory"
# Directories to not deploy in repository (must be relative path starting at root of repository)
IgnoreDirectories:
  - "Templates"
# Remote hosts to receive configurations
DeployerEndpoints:
  # name of each endpoint must have a matching directory name in the root of the git repo
  #examplehost:
  #  endpoint: "127.0.0.1"
  #examplehost2:
  #  endpoint: "127.0.0.2"
  #  endpointPort: 2022
  #  endpointUser: "deployer"
  #  SSHIdentityFile: "~/.ssh/private.key"
  #  SudoPassword: ""
  #  UseSSHAgent: false
  #  ignoreUniversalConfs: true
EOF
echo "[+] Successfully created controller configuration  in '$configFilePath'"

if [[ $AddSSHConfigHostsConfirmation == "y" ]]
then
	sshConfigHosts=$(egrep "^Host " ~/.ssh/config | sed 's/Host //g')
	for line in $sshConfigHosts
	do
		endpointName=$line
		endpoint=$(ssh -G $endpointName | egrep "^hostname " | sed 's/hostname //g')
		endpointPort=$(ssh -G $endpointName | egrep "^port "  | sed 's/port //g')
		endpointUser=$(ssh -G $endpointName | egrep "^user " | sed 's/user //g')
		if [[ $RemoteUsesDeployerServer == y ]]
		then
			endpointPort="2022"
			endpointUser="deployer"
		fi
		echo "  $endpointName:
    - endpoint: \"$endpoint\"
    - endpointPort: $endpointPort
    - endpointUser: \"$endpointUser\"" >> $configFilePath
		mkdir $RepositoryPath/$endpointName 2>/dev/null || logError "failed to create host $endpointName directory in git repository" "false"
	done
	echo "[+] Successfully added SSH client config hosts to '$configFilePath'"
fi

# Create first commit
GIT_AUTHOR_EMAIL=""
GIT_COMMITTER_EMAIL=""
git add . || logError "failed to git add, please fix error, disable hook, git add and commit" "false"
git commit -m 'Added controller configuration and universal directory' --author 'SCMPController <scmpc@localhost>' || logError "failed to git commit, please fix error, disable hook, and re-commit" "false"
echo "[+] Successfully committed controller files to new repository"

# Install git hook script
if [[ $InstallHookConfirmation == "y" ]]
then
	echo "#!/bin/bash
$executablePath --auto-deploy -c $configFilePath
" > $RepositoryPath/.git/hooks/post-commit || logError "failed to write post-commit hook to git repository" "true"
	chmod 750 $RepositoryPath/.git/hooks/post-commit
	echo "[+] Successfully created git post-commit hook in git repository"
fi

if [[ $installAAProfileConfirmation == "y" ]] then
	# Identify apparmor profile path
	ApparmorProfilePath=/etc/apparmor.d/$(echo $executablePath | sed 's|/|.|')
	#
	cat > "$ApparmorProfilePath" <<EOF
### Apparmor Profile for the Secure Configuration Management Controller
## This is a very locked down profile made for Debian systems
## Variables - add to if required
@{exelocation}=$executablePath
@{repolocations}={$RepositoryPath}
@{configlocations}={$configFilePath}
@{serverkeylocations}={$SSHIdentityPath}

@{profilelocation}=$ApparmorProfilePath
@{pid}={[1-9],[1-9][0-9],[1-9][0-9][0-9],[1-9][0-9][0-9][0-9],[1-9][0-9][0-9][0-9][0-9],[1-9][0-9][0-9][0-9][0-9][0-9],[1-4][0-9][0-9][0-9][0-9][0-9][0-9]}
@{home}={/root,/home/*}

## Profile Begin
profile SCMController @{exelocation} flags=(enforce) {
  # Receive signals
  signal receive set=(stop term kill quit int urg),
  # Send signals to self
  signal send set=(urg int) peer=SCMController,

  # Capabilities
  network netlink raw,
  network inet stream,
  network inet6 stream,
  unix (create) type=stream,
  unix (create) type=dgram,

  ## Startup Configurations needed
  @{configlocations} r,
  @{serverkeylocations} r,

  ## Program Accesses
  /sys/kernel/mm/transparent_hugepage/hpage_pmd_size r,

  ## Repository access
  # allow read/write for files in repository (write is needed for seeding operations)
  @{repolocations}/** rw,
  # allow locking in git's directory (for commit rollback on early error)
  @{repolocations}/.git/** k,
}
EOF
	#
	apparmor_parser -r $ApparmorProfilePath
	#
fi

echo "[+] New git repository created in $RepositoryPath with initial branch $BranchName and universal directory $UniversalDirectory"
echo "  [*] Don't forget to add the public key to all of your deployer endpoints:\n    $SSHPublicKey"

exit 0

# Controller Binary Embed #
__PAYLOAD_BEGINS__
