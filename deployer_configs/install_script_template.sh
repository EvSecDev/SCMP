#!/bin/bash
# Ensure script is run only in bash, required for built-ins (read, conditionals)
if [ -z "$BASH_VERSION" ]
then
	echo "This script must be run in BASH."
	exit 1
fi

# Only run script if running as root (or with sudo)
if [ "$EUID" -ne 0 ]
then
	echo "This script must be run with root permissions"
	exit 1
fi

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

# Check for commands
command -v echo >/dev/null || logError "echo command not found." "true"
command -v egrep >/dev/null || logError "egrep command not found." "true"
command -v grep >/dev/null || logError "grep command not found." "true"
command -v tar >/dev/null || logError "tar command not found." "true"
command -v sudo >/dev/null || logError "sudo command not found, it is needed for deployments." "true"
command -v ssh >/dev/null || logError "ssh command not found." "true"
command -v dirname >/dev/null || logError "dirname command not found." "true"
command -v mkdir >/dev/null || logError "mkdir command not found." "true"
command -v mv >/dev/null || logError "mv command not found." "true"
command -v rm >/dev/null || logError "rm command not found." "true"
command -v cat >/dev/null || logError "cat command not found." "true"
command -v base64 >/dev/null || logError "base64 command not found." "true"
command -v tail >/dev/null || logError "tail command not found." "true"
command -v ls >/dev/null || logError "ls command not found." "true"
command -v tr >/dev/null || logError "tr command not found." "true"
command -v ssh-keygen >/dev/null || logError "ssh-keygen command not found." "true"
command -v awk >/dev/null || logError "awk command not found." "true"
command -v sed >/dev/null || logError "sed command not found." "true"
command -v passwd >/dev/null || logError "passwd command not found." "true"
command -v print >/dev/null || logError "print command not found." "true"
command -v systemctl >/dev/null || logError "systemctl command not found." "true"
command -v objcopy >/dev/null || logError "objcopy (binutils on debian) command not found, it is needed for deployer updates" "true"

#### Installation
echo -e "\n========================================"
echo "      SCMPusher Deployer Installer      "
echo "========================================"
read -p " Press enter to begin the installation"
echo -e "========================================"

#{{DEFAULTS_PLACEHOLDER}}

#### User Choices
echo -e "Provide your choices for the installation. Press enter for the default.\n"

# Get user choice Exec Path only if exe from install archive is present
if [[ -f deployer ]]
then
	echo "[*] Enter the full path and file name where you would like the deployer executable to be"
	read -e -p "    (Default '$executablePath'): " UserChoice_executablePath
	if [[ $UserChoice_executablePath != "" ]]
	then
		executablePath=$UserChoice_executablePath
	fi
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
fi
# user supplied only if the key exists
if [[ -f $SSHPrivateKeyPath ]]
then
	UserSuppliedKey="true"
else
	UserSuppliedKey="false"
fi


# Listen addr
if [[ -f $configFilePath ]]
then
	# If a key is already in the config, set default
	regex_IP='ListenAddress: "(.*)"'
	ExistingListenIP=$(grep "ListenAddress" $configFilePath)
	if [[ $ExistingListenIP =~ $regex_IP ]]
	then
		SSHListenAddress="${BASH_REMATCH[1]}"
	fi
fi
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
if (( $SSHListenPort < 1024 ))
then
	logError "Port cannot be less than 1024, please choose another port" "true"
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
if [[ $AuthorizedUser == "root" ]]
then
	read -e -p "  [*] Are you sure you want to use a superuser for deployer? This better be for testing only... [y\N]" RootUser_Confirmation
	RootUser_Confirmation=$(echo $RootUser_Confirmation | tr [:upper:] [:lower:])
	if [[ $RootUser_Confirmation != "y" ]]
	then
		logError "refusing to run deployer as root user" "true"
	fi
fi

# Get existng auth key in config if it exists
if [[ -f $configFilePath ]]
then
	# If a key is already in the config, set default
	regex_authkey='-\s"(ssh.*)"'
	ExistingAuthKey=$(grep -A2 "AuthorizedKeys" $configFilePath)
	if [[ $ExistingAuthKey =~ $regex_authkey ]]
	then
		AuthorizedKeys="${BASH_REMATCH[1]}"
	fi
fi
# Get user choice for auth key
echo "[*] Enter the authorized SSH public key for the controller"
read -e -p "    (Default '$AuthorizedKeys'): " UserChoice_AuthorizedKey
if [[ $UserChoice_AuthorizedKey != "" ]]
then
	AuthorizedKeys=$UserChoice_AuthorizedKey
fi

# systemd service
echo "[*] Do you want to create the systemd service?"
read -e -p "    [y/N]: " CreateSystemdServiceConfirmation
CreateSystemdServiceConfirmation=$(echo $CreateSystemdServiceConfirmation | tr [:upper:] [:lower:])

# add sudo access to the user
echo "[*] Do you want to give sudo permissions to $AuthorizedUser?"
read -e -p "    [y/N]: " GiveSudoPermsConfirmation
GiveSudoPermsConfirmation=$(echo $GiveSudoPermsConfirmation | tr [:upper:] [:lower:])

# add apparmor profile
echo "[*] Do you want to install the apparmor profile?"
read -e -p "    [y/N]: " installAAProfileConfirmation
installAAProfileConfirmation=$(echo $installAAProfileConfirmation | tr [:upper:] [:lower:])
if [[ $installAAProfileConfirmation == "y" ]]
then
	command -v apparmor_parser >/dev/null || logError "apparmor_parser command not found, please install and retry." "true"
fi

# Ask for confirmation before continuing
echo -e "\n======================================"
echo "[*] Are the answers above all correct? Enter 'n' or nothing to exit"
read -e -p "    [y/N]: " ChoicesConfirmation
if [[ $ChoicesConfirmation != "y" ]]
then
	logError "aborting installation" "true"
fi
echo ""
echo "==== Starting Installation ===="

##
#### Actions on User Choices
##

# Setup User
if [[ $SetupUserConfirmation == "y" ]]
then
  # Check if user exists on this system (either as user or a group)
  if [[ $(egrep $AuthorizedUser /etc/passwd >/dev/null) ]]
  then
	echo "chosen username already exists on system"
  elif [[ $(egrep $AuthorizedUser /etc/group >/dev/null) ]]
  then
	echo "chosen username (as group) already exists on system"
  else
	# Add the user
	useradd --system --shell /usr/sbin/nologin -U $AuthorizedUser || logError "failed to add user $AuthorizedUser to system" "true"
	echo "[+] User $AuthorizedUser successfully created"
	# Change password for user
	echo "  [*] Please enter the password for the new user. This will be used for sudo escalation only, not for login."
	echo "  [*] This is the same password for all servers managed by SCMP, and is needed for the controller config (so remember it, or copy it somewhere safe)."
	passwd $AuthorizedUser || logError "failed to change password for user $AuthorizedUser" "true"
	echo "[+] Password for user $AuthorizedUser successfully changed"
  fi
fi

# Add Sudo Permissions
if [[ $GiveSudoPermsConfirmation == "y" ]]
then
  echo -e "\n# User for SCMP Deployer\n$AuthorizedUser ALL=(root:root) ALL, !/usr/bin/curl, !/usr/bin/wget, !/usr/bin/ncat, !/usr/bin/nc\n" >> /etc/sudoers || logError "failed to add sudo permissions for $AuthorizedUser" "true"
  echo "[+] Sudo permissions added to user $AuthorizedUser"
fi

# If service already exists, stop to allow new install over existing
if [[ -f $ServiceFilePath ]]
then
	systemctl stop $Service
fi

# Create SSH Key
if [[ $UserSuppliedKey == "false" ]]
then
	# generate new ssh key
	ssh-keygen -t ed25519 -N '' -C scmp/deployer -f "$SSHPrivateKeyPath" >/dev/null || logError "failed to generate private key" "true"
	rm $SSHPrivateKeyPath.pub 2>/dev/null
	echo "[+] Created new ssh key at $SSHPrivateKeyPath."
elif [[ $UserSuppliedKey == "true" ]]
then
	# check supplied key is present and valid
	chmod 600 $SSHPrivateKeyPath
	ssh-keygen -y -f $SSHPrivateKeyPath >/dev/null || logError "failed to validate ssh private key in $SSHPrivateKeyPath" "false"
	echo "[+] Found existing ssh key at $SSHPrivateKeyPath... using it"
fi
chown $AuthorizedUser:$AuthorizedUser $SSHPrivateKeyPath || logError "failed to change ownership of ssh private key at $SSHPrivateKeyPath, please do it yourself" "false"
chmod 400 $SSHPrivateKeyPath || logError "failed to change permissions of ssh private key at $SSHPrivateKeyPath, please do it yourself" "false"

# Move deployer binary into place
if [[ -f deployer ]]
then
  cp deployer $executablePath || logError "failed to move executable" "true"
  chown root:root $executablePath || logError "failed to change ownership of executable at $executablePath, please do it yourself" "false"
  chmod 755 $executablePath || logError "failed to change permissions of executable at $executablePath, please do it yourself" "false"
  echo "[+] Successfully extracted deployer binary to $executablePath"
else
  logError "cannot find deployer binary in current working directory for installation" "true"
fi

# Deployer updater binary
if [[ -f updater ]]
then
  cp updater $updateProgramPath || logError "failed to move updater" "true"
  chown root:root $updateProgramPath || logError "failed to change ownership of updater executable at $updateProgramPath, please do it yourself" "false"
  chmod 755 $updateProgramPath || logError "failed to change permissions of updater executable at $updateProgramPath, please do it yourself" "false"
  echo "[+] Successfully extracted deployer updater to $updateProgramPath"
else
  logError "cannot find updater binary in current working directory for installation" "true"
fi

# Install apparmor profile
if [[ $installAAProfileConfirmation == "y" ]]
then
  cat > "$ApparmorProfilePath" <<EOF
#{{AAPROF_PLACEHOLDER}}
EOF
	chmod 644 "$ApparmorProfilePath"
	chown root:root "$ApparmorProfilePath"
	apparmor_parser -r "$ApparmorProfilePath"
fi

# Put config in user choosen location
cat > "$configFilePath" <<EOF
#{{CONFIG_PLACEHOLDER}}
EOF
echo "[+] Successfully created deployer configuration at $configFilePath"

# Setup Systemd Service
if [[ $CreateSystemdServiceConfirmation == "y" ]]
then
  cat > "$ServiceFilePath" <<EOF
#{{SYSTEMD_SERVICE_PLACEHOLDER}}
EOF
  # reload units, enable, and start
  systemctl daemon-reload || logError "failed to reload systemd daemon for new unit" "true"
  systemctl enable $Service || logError "failed to enable systemd service" "false"
  systemctl start $Service || logError "failed to start systemd service" "true"
  echo "[+] Systemd service installed, enabled, and started."
fi

# Cleanup
rm updater deployer

echo "==== Finished Installation ===="
echo ""
rm $0 2>/dev/null
exit 0
