#!/bin/bash
#### Error handling

logError() {
	local errorMessage=$1
	local exitRequested=$2

	# print the error to the user
	echo "[-] Error: $errorMessage"

	if $exitRequested == "true"
	then
		exit 1
	fi
}
#### Pre Checks

# Ensure script is run in bash
if [ -z "$BASH_VERSION" ]
then
	logError "This script must be run in Bash." "true"
fi

# Only run script if running as root
if [ "$EUID" -ne 0 ]
then
	logError "This script must be run with root permissions" "true"
fi

# Check for commands
command -v egrep >/dev/null || logError "egrep command not found." "true"
command -v tar >/dev/null || logError "tar command not found." "true"
command -v sudo >/dev/null || logError "sudo command not found, it is needed for deployments." "true"
command -v ssh >/dev/null || logError "ssh command not found." "true"
command -v dirname >/dev/null || logError "dirname command not found." "true"
command -v mkdir >/dev/null || logError "mkdir command not found." "true"
command -v echo >/dev/null || logError "echo command not found." "true"
command -v mv >/dev/null || logError "mv command not found." "true"
command -v rm >/dev/null || logError "rm command not found." "true"
command -v cat >/dev/null || logError "cat command not found." "true"
command -v base64 >/dev/null || logError "base64 command not found." "true"
command -v ls >/dev/null || logError "ls command not found." "true"
command -v tr >/dev/null || logError "tr command not found." "true"
command -v ssh-keygen >/dev/null || logError "ssh-keygen command not found." "true"
command -v awk >/dev/null || logError "awk command not found." "true"

#### Vars

#### Installation
echo "      SCMPusher Deployer Installer      "
echo "========================================"
read -p " Press enter to begin the installation"
echo "========================================"

# Default choices
executablePath="/usr/local/bin/scmdeployer"
configFilePath="/etc/scmpd.yaml"
SSHPrivateKeyPath="/usr/local/share/scmp_ssh.key"
SSHListenAddress="0.0.0.0"
SSHListenPort="2022"
AuthorizedUser="deployer"
AuthorizedKeys=""

#### User Choices
echo -e "Provide your choices for the installation. Press enter for the default.\n"

# Exec Path
echo "[*] Enter the full path and file name where you would like the deployer executable to be"
read -e -p "    (Default '$executablePath'): " UserChoice_executablePath
if [[ $UserChoice_executablePath != "" ]]
then
	executablePath=$UserChoice_executablePath
fi

# Config path
echo "[*] Enter the full path and file name where you would like the deployer config to be"
read -e -p "    (Default '$configFilePath'): " UserChoice_configFilePath
if [[ $UserChoice_configFilePath != "" ]]
then
	configFilePath=$UserChoice_configFilePath
fi

# ssh key path
echo "[*] Enter the full path and file name to the SSH private key for the deployer (ed25519 key type required)"
read -e -p "    (Default '$SSHPrivateKeyPath'): " UserChoice_SSHPrivateKeyPath
if [[ $UserChoice_SSHPrivateKeyPath != "" ]]
then
	SSHPrivateKeyPath=$UserChoice_SSHPrivateKeyPath
	# user supplied only if the key exists
	if [[ $(ls $SSHPrivateKeyPath 2>&1 1>/dev/null) ]]
	then
		UserSuppliedKey="true"
	else
		UserSuppliedKey="false"
	fi
fi

# Listen addr
echo "[*] Enter the IP address for the SSH Deployer Server to listen on"
read -e -p "    (Default '$SSHListenAddress'): " UserChoice_SSHListenAddress
if [[ $UserChoice_SSHListenAddress != "" ]]
then
	SSHListenAddress=$UserChoice_SSHListenAddress
fi

# Listen Port
echo "[*] Enter the port for the SSH Deployer Server to listen on"
read -e -p "    (Default '$SSHListenPort): " UserChoice_SSHListenPort
if [[ $UserChoice_SSHListenPort != "" ]]
then
	SSHListenPort=$UserChoice_SSHListenPort
fi

# SSH user setup
echo "[*] Do you want to create a new user for the deployer (Server will run as user and controller will authenticate as this user)?"
read -e -p "    [y/N]: " SetupUserConfirmation
SetupUserConfirmation=$(echo $SetupUserConfirmation | tr [:upper:] [:lower:])

# SSH username
echo "[*] Enter the authorized SSH username that the controller will use"
read -e -p "    (Default '$AuthorizedUser'): " UserChoice_AuthorizedUser
if [[ $UserChoice_AuthorizedUser != "" ]]
then
	AuthorizedUser=$UserChoice_AuthorizedUser
fi

# systemd service
echo "[*] Do you want to create the systemd service?"
read -e -p "    [y/N]: " CreateSystemdServiceConfirmation
CreateSystemdServiceConfirmation=$(echo $CreateSystemdServiceConfirmation | tr [:upper:] [:lower:])

# enable systemd service
echo "[*] Do you want to start the service on boot?"
read -e -p "    [y/N]: " EnableSystemdServiceConfirmation
EnableSystemdServiceConfirmation=$(echo $EnableSystemdServiceConfirmation | tr [:upper:] [:lower:])

# add sudo access to the user
echo "[*] Do you want to give sudo permissions to $AuthorizedUser?"
read -e -p "    [y/N]: " GiveSudoPermsConfirmation
GiveSudoPermsConfirmation=$(echo $GiveSudoPermsConfirmation | tr [:upper:] [:lower:])

# Ask for confirmation before continuing
echo -e "\n======================================"
echo "[*] Are the answers above all correct? Enter 'n' or nothing to exit"
read -e -p "    [y/N]: " ChoicesConfirmation
if [[ $ChoicesConfirmation != "y" ]]
then
	logError "aborting installation" "true"
fi

#### Actions on User Choices

if $AuthorizedUser == "root"
then
	logError "refusing to run deployer as root user" "true"
fi

# Setup User
if [[ $SetupUserConfirmation == "y" ]]
then
	# Check if user exists on this system (either as user or a group)
	if [[ $(egrep $AuthorizedUser /etc/passwd >/dev/null) ]]
	then
		echo "chosen username already exists on system, choose another."
		exit 1
	fi
	if [[ $(egrep $AuthorizedUser /etc/group >/dev/null) ]]
	then
		echo "chosen username already exists on system, choose another."
		exit 1
	fi

	# Add the user
	useradd --system --shell /usr/sbin/nologin -U $AuthorizedUser || logError "failed to add user $AuthorizedUser to system" "true"

	echo "[+] User %AuthorizedUser successfully created"
	# Change password for user
	echo "  [*] Please enter the password for the new user. This will be used for sudo escalation only, not for login."
	echo "  [*] This is the same password for all servers managed by SCMP, and is needed for the controller config (so remember it, or copy it somewhere safe)."
	passwd $AuthorizedUser || logError "failed to change password for user $AuthorizedUser" "true"
	echo "[+] Password for user %AuthorizedUser successfully changed"
fi

# Add Sudo Permissions
if [[ $GiveSudoPermsConfirmation == "y" ]]
then
	echo -e "\n# User for SCMP Deployer\n$AuthorizedUser ALL=(root:root) ALL, !/usr/bin/curl, !/usr/bin/wget, !/usr/bin/ncat, !/usr/bin/nc\n" >> /etc/sudoers || logError "failed to add sudo permissions for $AuthorizedUser" "true"
	echo "[+] Sudo permissions added to user $AuthorizedUser"
fi

# Setup Systemd Service
if [[ $CreateSystemdServiceConfirmation == "y" ]]
then
	ServiceDir="/etc/systemd/system"
	Service="scmpd.service"
	ServiceFilePath="$ServiceDir/$Service"

	echo "[Unit]
Description=SCM Deployer Agent
After=network.target

[Service]
ExecStart=$executablePath --start-server -c $configDstPath
User=$AuthorizedUser
Group=$AuthorizedUser
Type=exec
RestartSec=1min
Restart=always

[Install]
WantedBy=multi-user.target
" > $ServiceFilePath || logError "failed to write systemd service file at $ServiceFilePath" "true"

	# reload units
	systemctl daemon-reload || logError "failed to reload systemd daemon for new unit" "true"

	# Start on boot if requested
	if [[ $EnableSystemdServiceConfirmation == "y" ]]
	then
		systemctl enable $Service || logError "failed to enable systemd service" "false"
		echo "[+] Systemd service installed and enabled, start it with 'systemctl start $Service'"
	else
		echo "[+] Systemd service installed, start it with 'systemctl start $Service'"
	fi
fi

# Create SSH Key
if [[ $UserSuppliedKey != "true" ]]
then
	# generate new ssh key
	if [[ -f $SSHPrivateKeyPath ]]
	then
		rm $SSHPrivateKeyPath 2>/dev/null
	fi
	ssh-keygen -t ed25519 -N '' -C scmp/deployer -f $SSHPrivateKeyPath >/dev/null || logError "failed to generate private key" "true"
	rm $SSHPrivateKeyPath.pub 2>/dev/null
	echo "[+] Created new ssh key at $SSHPrivateKeyPath."
elif [[ $UserSuppliedKey == "true" ]]
then
	# check supplied key is present and valid
	ssh-keygen -y -f $SSHPrivateKeyPath || logError "failed to find ssh private key in $SSHPrivateKeyPath" "false"
	echo "[+] Found existing ssh key at $SSHPrivateKeyPath... using it"
fi

# Extract embedded exe from this script and put in user choice location
PAYLOAD_LINE=$(awk '/^__PAYLOAD_BEGINS__/ { print NR + 1; exit 0; }' $0)
executableDirs=$(dirname $executablePath 2>/dev/null || logError "failed to determine executable parent directories" "true")
mkdir -p $executableDirs 2>/dev/null || logError "failed to create executable parent directory" "true"
tail -n +${PAYLOAD_LINE} $0 | base64 -d | tar -zpvx -C $executableDirs || logError "failed to extract embedded executable" "true"
mv $executableDirs/deployer $executablePath || logError "failed to move executable" "true"
echo "[+] Successfully extracted deployer binary to $executablePath"

# Put config in user choosen location
echo "SSHServer:
  ListenAddress: "$SSHListenAddress"
  ListenPort: "$SSHListenPort"
  SSHPrivKeyFile: "$SSHPrivateKeyPath"
  AuthorizedUser: "$AuthorizedUser"
  AuthorizedKeys:
    - ""
" > $configFilePath || logError "failed to write config file to $configFilePath" "true"
echo "[+] Successfully created deployer configuration at $configFilePath"

echo "== Finished Installation =="

exit 0

# Deployer Binary Embed #
__PAYLOAD_BEGINS__
