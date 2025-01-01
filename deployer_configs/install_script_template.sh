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

#### Help menu
function usage {
	echo "Usage $0

Options:
  -a   Automatic installation using embedded variables for choices
"
}

#### Pre Checks - ensure required commands are present
command -v echo >/dev/null || logError "echo command not found." "true"
command -v egrep >/dev/null || logError "egrep command not found." "true"
command -v grep >/dev/null || logError "grep command not found." "true"
command -v sudo >/dev/null || logError "sudo command not found, it is needed for deployments." "true"
command -v dirname >/dev/null || logError "dirname command not found." "true"
command -v mkdir >/dev/null || logError "mkdir command not found." "true"
command -v mv >/dev/null || logError "mv command not found." "true"
command -v rm >/dev/null || logError "rm command not found." "true"
command -v cat >/dev/null || logError "cat command not found." "true"
command -v ls >/dev/null || logError "ls command not found." "true"
command -v tr >/dev/null || logError "tr command not found." "true"
command -v ssh-keygen >/dev/null || logError "ssh-keygen command not found." "true"
command -v passwd >/dev/null || logError "passwd command not found." "true"
command -v usermod >/dev/null || logError "passwd command not found." "true" 
command -v systemctl >/dev/null || logError "systemctl command not found." "true"
command -v objcopy >/dev/null || logError "objcopy (binutils on debian) command not found, it is needed for deployer updates" "true"

#{{DEFAULTS_PLACEHOLDER}}

manualInstallation() {
	#### User Choices
	echo -e "\n========================================"
	echo "      SCMPusher Deployer Installer      "
	echo "========================================"
	read -p " Press enter to begin the installation"
	echo -e "========================================"
	echo -e "Provide your choices for the installation. Press enter for the default.\n"

	# SSH key path
	echo "[*] Enter the full path and file name to the SSH private key for the deployer"
	read -e -p "    (Default '$SSHPrivateKeyPath'): " UserChoice_SSHPrivateKeyPath
	if [[ $UserChoice_SSHPrivateKeyPath != "" ]]
	then
		SSHPrivateKeyPath=$UserChoice_SSHPrivateKeyPath
	fi

	# Listen address
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

	# Systemd service
	echo "[*] Do you want to create the systemd service?"
	read -e -p "    [y/N]: " CreateSystemdServiceConfirmation
	CreateSystemdServiceConfirmation=$(echo $CreateSystemdServiceConfirmation | tr [:upper:] [:lower:])

	# Add sudo access to the user
	echo "[*] Do you want to give sudo permissions to $AuthorizedUser?"
	read -e -p "    [y/N]: " GiveSudoPermsConfirmation
	GiveSudoPermsConfirmation=$(echo $GiveSudoPermsConfirmation | tr [:upper:] [:lower:])

	# Add apparmor profile
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
}

#### START
automaticInstallation="false"

# Argument parsing
while getopts 'ah' opt
do
	case "$opt" in
	  'a')
	    automaticInstallation="true"
	    ;;
	  'h')
	    usage
	    exit 0
 	    ;;
	esac
done

# Ask for user choices 
if [[ $automaticInstallation == false ]]
then
	manualInstallation
fi

#### Installation Actions
echo -e "\n==== Starting Installation ===="

# Setup User
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

	# No manual hash specified, ask for the password
	if [[ -z $UserPasswordShadowHash ]]
	then
		echo "  [*] Please enter the password for the new user. This will be used for sudo escalation only, not for login."
		echo "  [*] This is the same password for all servers managed by SCMP, and is needed for the controller config (so remember it, or copy it somewhere safe)."
		passwd $AuthorizedUser || logError "failed to change password for user $AuthorizedUser" "true"
	else
		usermod --password "$UserPasswordShadowHash" "$AuthorizedUser" || logError "failed to change password for user $AuthorizedUser" "true"
	fi

	echo "[+] Password for user $AuthorizedUser successfully changed"
fi

# Add Sudo Permissions
if [[ $GiveSudoPermsConfirmation == "y" ]]
then
	echo -e "\n# User for SCMP Deployer\n$AuthorizedUser ALL=(root:root) ALL, !/usr/bin/curl, !/usr/bin/wget, !/usr/bin/ncat, !/usr/bin/nc\n" >> /etc/sudoers || logError "failed to add sudo permissions for $AuthorizedUser" "true"
	echo "[+] Sudo permissions added to user $AuthorizedUser"
fi

# Create SSH Key or validate existing key
if [[ -f $SSHPrivateKeyPath ]]
then
	# Check supplied key is present and valid
	chmod 600 $SSHPrivateKeyPath
	ssh-keygen -y -f $SSHPrivateKeyPath >/dev/null || logError "failed to validate ssh private key in $SSHPrivateKeyPath" "false"
	echo "[+] Found existing ssh key at $SSHPrivateKeyPath... using it"
else 
	# Generate new ssh key
	ssh-keygen -t ed25519 -N '' -C scmp/deployer -f "$SSHPrivateKeyPath" >/dev/null || logError "failed to generate private key" "true"
	rm $SSHPrivateKeyPath.pub 2>/dev/null
	echo "[+] Created new ssh key at $SSHPrivateKeyPath."
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
	# Write to apparmor profile file
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
	# If service already exists, stop to allow new install over existing
	if [[ -f $ServiceFilePath ]]
	then
		systemctl stop $Service
	fi
	
	# Write to service file
	cat > "$ServiceFilePath" <<EOF
#{{SYSTEMD_SERVICE_PLACEHOLDER}}
EOF

	# Reload units, enable, and start
	systemctl daemon-reload || logError "failed to reload systemd daemon for new unit" "true"
	systemctl enable $Service || logError "failed to enable systemd service" "false"
	systemctl start $Service || logError "failed to start systemd service" "true"
	echo "[+] Systemd service installed, enabled, and started."
fi

# Cleanup
rm updater deployer
rm deployer_package*tar.gz 2>/dev/null
rm $0 2>/dev/null

echo -e "==== Finished Installation ====\n"
exit 0