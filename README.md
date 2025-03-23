# Secure Configuration Management Program (SCMP)

## Description

A secure and automated configuration management terminal-based tool backed by git to centrally control and push configuration files to Unix servers through SSH.

This program is designed to assist and automate a Linux administrators job functions by centrally allowing them to edit, version control, and deploy changes to configuration files of remote Linux systems.
This program is NOT intended as a configuration management system (like Terraform), but rather a CLI tool to replace the manual process of SSH'ing into many remote servers to manage configuration files.

The controller utilizes a local git repository of a specific structure to track configuration files that should be applied to every host administered, and specific config overrides as well as host-specific configurations.
The configuration for the controller utilizes a semi-standard `~/.ssh/config` that you would normally use with any other SSH client.
The 'semi-standard' part of this is the inclusion of some advanced configuration options to better integrate with git and deployment activities.
Fear not, you can use your `~/.ssh/config` with the controller and a regular SSH client at the same time.

For sudo passwords, this program utilizes a simple password vault file stored where ever you specify.
This vault stores the password per host and is manipulated through controller (add/change/remove).
This is intended to facilitate deployments to a large number of hosts with potentially different passwords. With the vault, your provide the master password only once.
The vault is protected by an AEAD cipher (chacha20poly1305) and derives the key via Argon2 from your master password.

Using the Go x/crypto/ssh package, this program will SSH into the hosts defined in the configuration file and write the relevant configurations as well as handle the reloading of the associated service/program if required.
  The deployment method is currently only SSH by key authentication using password sudo for remote commands (password login authentication is currently not supported).

- In deploy changes mode, you can choose a specific commit ID (or specify none and use the latest commit) from your repository and deploy the changed files in that specific commit to their designated remote hosts.
- In deploy failure mode, the program will read the last failure json (if present) and extract the commitid, hosts, and files that failed and attempt to redeploy.
- In deploy all mode, with a comma separated list of hosts, you can deploy every relevant file in the repo to the chosen hosts for a given commit (usually, the head commit).

Although this program does need permissions on remote systems for writing system-wide configuration files and potentially restarting services, it does NOT need to SSH as root.
In general, it is recommended to use some or all of these below security precautions.

- Sudo access that requires a password.
- Only allowing your user sudo access for the standard commands (listed below in dependencies section) and your reload commands.
- Using network level host IP authentication (such as IPsec AH)
- Using the supplied apparmor profile for the controller.
- Regular encrypted backups of git repository

Below you can find the recommended setup for the remote servers, and how to configure the remote host to have the least privileges possible to fulfill the functions of this program.

If you like what this program can do or want to expand functionality yourself, feel free to submit a pull request or fork.

## Capabilities Overview

### What it can do

- Deployments
  - Deploy changed configurations automatically via git post-commit hook or manually via specifying a commit hash
  - Deploy all (or a subset of) tracked files
  - Deploy individual/lists/groups of files to individual/lists/groups of hosts
  - Deployment test run using single host (use `--max-conns 1 -r HOST`)
  - Exclude hosts from deployments (use config option `DeploymentState offline`)
  - Ad-hoc override host exclusion from deployments (use `--ignore-deployment-state`)
  - Run a linear series of commands prior to any deployment actions per file/directory (part of JSON metadata header)
  - Run a linear series of commands to enable/reload/start services associated with files/directories (part of JSON metadata header)
    - Option to temporarily disable globally for a deployment
  - Run a linear series of commands to install services associated with files/directories (part of JSON metadata header)
  - Easy retry of deployment failures with a single argument
  - Fail-safe file deployment - automatic restore of previous file version if any remote failure is encountered
- File/Directory Management
  - Create/modify files/file content and directories
  - Modify permissions, owner, and group of files and directories
  - Removing 'managed' files and directories
  - Group files together to apply to multiple hosts
  - Options to ignore specific directories in the repository
  - Track binary/artifact files (executables, images, videos, documents)
  - Support standard ASCII naming
- Host Management
  - Use standard SSH client config to management endpoints
  - Ability to mark individual hosts as offline to prevent deployments to that host
  - Apply file groups to distribute single file version to all or a subset of all hosts
- SSH
  - Password-based login
  - Key-based authentication (by file or ssh-agent, per host or all hosts)
  - Concurrent connections (and option to limit/disable concurrency)
  - Password-based Sudo command escalation (and non-sudo actions via explicit argument)
  - Encrypted credential caching for login/sudo passwords
- Controller Functionality
  - Create new repositories
  - Collect configurations from existing systems to bootstrap the local repository
  - Use file input to any of the host/file arguments using a file URI scheme (like `file:///absolute/path/file`, `file://relative/path/file`)

### What it can NOT do

- File/Directory Management
  - File/Directory names containing newlines or DEL characters
  - File/Directory names containing non-printable ASCII as well as non-ASCII characters
  - Handle some special files (device, pipes, sockets, ect.)
- SSH
  - 2FA (TOTP) logins
  - Use Control Sockets
  - Use any form of client forwarding (tunnels, x11, agents)

### Dependencies

- Remote Host Requirements:
  - OpenSSH Server
  - Commands: `ls, stat, rm, mv, cp, ln, rmdir, mkdir, chown, chmod, sha256sum`
- Local Host Requirements:
  - Unix file paths

### Controller Help Menu

```bash
Secure Configuration Management Program (SCMP)
  Deploy configuration files from a git repository to Linux servers via SSH
  Deploy ad-hoc commands and scripts to Linux servers via SSH

  Options:
    -c, --config </path/to/ssh/config>             Path to the configuration file
                                                   [default: ~/.ssh/config]
    -d, --deploy-changes                           Deploy changed files in the specified commit
                                                   [commit default: head]
    -a, --deploy-all                               Deploy all files in specified commit
                                                   [commit default: head]
    -f, --deploy-failures                          Deploy failed files/hosts using
                                                   failtracker file from last failed deployment
    -e, --execute <"command"|file:///>             Run adhoc single command or upload and
                                                   execute the script on remote hosts
    -r, --remote-hosts <host1,host2,...|file:///>  Override hosts to connect to for deployment
                                                   or adhoc command/script execution
    -R, --remote-files <file1,file2,...|file:///>  Override file(s) to retrieve using seed-repository
                                                   Also override default remote path for script execution
    -l, --local-files <file1,file2,...|file:///>   Override file(s) for deployment
                                                   Must be relative file paths from inside the repository
    -C, --commitid <hash>                          Commit ID (hash) of the commit to
                                                   deploy configurations from
    -T, --dry-run                                  Does everything except start SSH connections
                                                   Prints out deployment information
    -m, --max-conns <15>                           Maximum simultaneous outbound SSH connections
                                                   [default: 10] (1 disables concurrency)
    -p, --modify-vault-password <host>             Create/Change/Delete a hosts password in the
                                                   vault (will create the vault if it doesn't exist)
    -n, --new-repo </path/to/repo>:<branch>        Create a new repository at the given path
                                                   with the given initial branch name
    -s, --seed-repo                                Retrieve existing files from remote hosts to
                                                   seed the local repository (Requires '--remote-hosts')
        --git-add <dir|file>                       Add files/directories/globs to git worktree
                                                   Required for artifact tracking feature
        --git-status                               Check current worktree status
                                                   Prints out file paths that differ from clean worktree
        --git-commit <'message'|file:///>          Commit changes to git repository with message
                                                   File contents will be read and used as message
        --allow-deletions                          Allows deletions (remote files or vault entires)
                                                   Only applies to '--deploy-changes' or '--modify-vault-password'
        --install                                  Runs installation commands in config files metadata JSON header
                                                   Commands are run before file deployments (before checks)
        --force                                    Ignores checks and runs atomically
                                                   Forces writes and reloads of deployment files
        --disable-reloads                          Disables execution of reload commands for this deployment
                                                   Useful to write configs that normally need reloads without running them
        --disable-privilege-escalation             Disables use of sudo when executing commands remotely
                                                   All commands will be run as the login user
        --ignore-deployment-state                  Ignores the current deployment state in the configuration file
                                                   For example, will deploy to a host marked as offline
        --regex                                    Enables regular expression parsing for specific arguments
                                                   Supported arguments: '-r', '-R', '-l'
        --log-file                                 Write out events to log file
                                                   Output verbosity follows program-wide '--verbose'
    -t, --test-config                              Test controller configuration syntax
                                                   and configuration option validity
    -v, --verbose <0...5>                          Increase details and frequency of progress messages
                                                   (Higher is more verbose) [default: 1]
    -h, --help                                     Show this help menu
    -V, --version                                  Show version and packages
        --versionid                                Show only version number

  Report bugs to: dev@evsec.net
  SCMP home page: <https://github.com/EvSecDev/SCMP>
  General help using GNU software: <https://www.gnu.org/gethelp/>
```

## Setup and Configuration

1. Create an SSH private key (Alternatively, use any existing one)
    - `ssh-keygen -t ed25519 -N '' -C scmp/controller -f controller_ssh`
2. Move the controller executable to your desired location within your path. Example:
    - `mv controller_v* /usr/local/bin/controller`
3. To generate a new git repository, run this command:
    - `controller --new-repo /path/to/you/new/repo:main`
    - 3a) **Optional**: If you want a sample configuration file, run this command
      - `controller --install-default-config`
    - 3b) **Optional**: If you want to install the AppArmor profile, run this command
      - `sudo controller --install-apparmor-profile`
    - 3c) **Optional**: If you want bash auto-completion for the controller arguments, see the snippet to add to your `~/.bashrc` in the Notes section
4. Configure the SSH configuration file for all the remote Linux hosts you wish to manage (see comments in config for what the fields mean)
5. Done! Proceed to remote preparation

### Remote Preparation

1. Create a user that can log into SSH and use Sudo
   - `useradd --create-home --user-group deployer`
   - `passwd deployer`
2. Add the SSH public key **from the controller installation script** to the users home directory `authorized_keys` file
   - `mkdir -p /home/deployer/.ssh && echo "ssh-ed25519 AAAADEADBEEFDEABEEFDEADBEEF scmp/controller" >> /home/deployer/.ssh/authorized_keys`
3. Modify `/etc/sudoers` with the below line to allow your new user to run Sudo commands with a password
   - `deployer ALL=(root:root) ALL`
   - **Optionally**, restrict the commands your new user can run in the sudoers file to the following:
     - ls, rm, cp, ln, rmdir, mkdir, chown, chmod, sha256sum, and any reload commands you need (systemctl, sysctl, ect.)
     - `deployer ALL=(root:root) PASSWD: /usr/bin/ls, /usr/bin/rm, /usr/bin/cp, /usr/bin/ln, /usr/bin/rmdir, /usr/bin/mkdir, /usr/bin/chown, /usr/bin/chmod, /usr/bin/sha256sum, /usr/bin/systemctl`

### Bootstrapping the Repository

So, what if you already have servers with potentially hundreds of configuration files spread throughout the system?
Well, fear not, for there is a SSH client built into the controller as an easier method of transferring and formatting new files.
The client will permit you to select files on a remote system and automatically format and add them in the correct location to the local repository.

If you already know which files you want from a remote host/hosts, then you can use `--remote-files file:///path/to/textfile` to give the controller a list of files to download from the remote host.

This feature requires that you have installed controller and configured the SSH configuration file with the hosts you want to manage.
It also requires that the remote host is setup as described in the SSH config (port is open, user is allowed, ect.)

The interface you will be using for this feature is extremely barebones. It looks like this:

```bash
==== Secure Configuration Management Repository Seeding ====
============================================================
1 bin        7  initrd.img.old 13 opt/   19 sys/
2 boot/      8  lib            14 proc/  20 tmp/
3 dev/       9  lib64          15 root/  21 usr/
4 etc/       10 lost+found/    16 run/   22 var/
5 home/      11 media/         17 sbin   23 vmlinuz
6 initrd.img 12 mnt/           18 srv/   24 vmlinuz.old 
============================================================
         Select File    Change Dir ^v  Exit
        [ # # ## ### ]  [ c0 ] [ c# ]  [ ! ]
hostname:/# Type your selections: _
```

If you wanted to change directories to `/etc/`, you'd type this and press enter:

`hostname:/# Type your selections: c4`

If you wanted to select files `vmlinuz`, `initrd.img.old`, `initrd.img`, and then exit you'd type this and press enter:

`hostname:/# Type your selections: 23 7 6 !`

If you were in `/etc/` and wanted to move up one directory, you'd type this and press enter:

`hostname:/# Type your selections: c0`

The shortcuts will be listed below every directory so you won't need to remember them.
You can type as many or as little options as you wish in any order, they will all be added.
Selected files will be saved before changing directories, so you can navigate the whole remote host file system saving files you want as you go.

Once you have selected all your files and typed `!`, you will be asked (file by file) if the config requires reload commands, and if so, you can provided them one per line.
The controller will then take all the files and write them to their respective host directories in the local repository copying the remote host file path.

The structure of the local repository is supposed to be a replica of the remote server filesystem, to facilitate editing and organizing files as you normally would on the remote system.

```sh
-----------------------------
-> RepositoryDirectory
  -> UniversalConfs
    -> etc
      -> resolv.conf
      -> hosts
      -> motd
    -> home
      -> user
        -> .bashrc
        -> .ssh
          -> authorized_keys
  -> UniversalConfs_NGINX
     -> etc
       -> nginx
         -> nginx.conf
         -> snippets
           -> ssl_params.conf
  -> Host1
    -> etc
      -> rsyslog.conf
      -> motd
    -> opt
      -> program1
        -> .env
  -> Host2
    -> etc
      -> network
        -> interfaces
      -> hostname
      -> crontab
    -> home
      -> user
        -> .bashrc
-----------------------------
```

## NOTES

### Universal Configs

This program's objective of simplifying configuration management would not be complete without the ability to deploy the same file to all or groups of hosts.
To this end, there are two features that make this possible: UniversalConfs and UniversalGroups.

UniversalConfs is a directory in the root of your repository that will contain a filesystem-like directory structure underneath it.
Configuration files in this directory will be applicable for deployment to all hosts.
If a particular host should need a slightly different version of the UniversalConf config, then a file with an identical path and name should be put under the host directory to stop that host from using the universal config.
If a particular host should not ever use the UniversalConf configs, then the config option `ignoreUniversalConfs` should be set to true under that particular host in the main config.

UniversalGroups is a set of directories that will only apply to a subset of hosts.
The functionality is identical to the UniversalConfs directory, but will only apply to hosts that are apart of the group.
You can specify the directory name and the hosts that should use the directory in the SSH config.

### Directory Management

The version control and deployment of directory and directory metadata is split in two.
The existence of a directory/directory structure will imply the creation of the same structure on the remote host.
Removal of managed directories in the repository will only remove the remote directory if the remote directory is empty.

Metadata of directories is handled through a special JSON file that lives directly under the directory it applies to.
The file name is static and will always need to be `.directory_metadata_information.json`

The JSON is the same as the metadata header in files with no reload section:

```json
{
  "FileOwnerGroup": "www-data:www-data",
  "FilePermissions": 755
}
```

This metadata file is not seeded and is manually created.
This feature is not meant to be used everywhere. The default is the remote hosts default (usually `root:root` `rwxr-xr-x`).
This metadata file should only be used where custom permissions are absolutely required.

### File transfers

File transfers for this program are done using SCP and are limited to 90 seconds per file.
Something to keep in mind, your end to end bandwidth for a deployment will determine how large of a file can be transferred in that time.

### Artifact Files (External Git Content)

Binary files and other non-text files (artifacts) are not great at being tracked by git.
Due to this, there is a workaround to "version control" binary files while still being tied into SCMP deployments.

This feature requires three things:

- The file in the SCMP git repository needs an extra field in the metadata header JSON: `ExternalContentLocation`:
  - Example: `"ExternalContentLocation":"file:///absolute/path/to/actual/binary/file"`
- The file in the SCMP git repository can be named whatever it needs to be, but requires `.remote-artifact` file extension.
  - The extension is just for local identification, the extension is removed prior to deployment.
- Use the built-in controller git add argument `--git-add`
  - This is how the artifact files are tracked by git.

Any content past the metadata header in the `.remote-artifact` file is used to store the hash of the artifact file content.
This hash is updated before every commit to ensure updates remotely are tracked in git without tracking the binary's actual content

You might be asking, how does the git repository know when your binary file has changed?
Since the binary file is not tracked directly in git, any `.remote-artifact` files will be flagged for inspection when using controller's `--git-add` argument.
The controller will follow the file path you give in the `ExternalContentLocation` and hash the current artifact file.
Once the artifact pointer file is flagged as changed by git, the normal deployment process takes place (with the caveat that content loading is done using the `ExternalContentLocation`)

Due to this system, binary files do take up extra processing power and memory space since changes are tracked at runtime.

Only `file://` (local) URIs are supported for the `ExternalContentLocation` field currently.

### Command Macros (Internal Variables)

Certain macros are supported in the install/check/reload command strings.
These macros are replaced with known values during predeployment file processing.

Notes:

- Macro names are case-sensitive.
- Macros inside of double quotes will throw a JSON formatting error

Example of expansion given input of `Server01/etc/nginx/nginx.conf` (other values are retrieved from `.ssh/config`):

```text
{@FILEPATH}      -> /etc/nginx/nginx.conf
{@FILEDIR}       -> /etc/nginx
{@FILENAME}      -> nginx.conf
{@HOSTNAME}      -> Server01
{@HOSTLOGINUSER} -> user1
{@HOSTIP}        -> 192.168.0.1
{@HOSTPORT}      -> 22
```

### Install commands

Commands in this metadata JSON array are run only by using the controller argument `--install`.
This feature is meant to provide a mechanism to initialize a service prior to deploying the file.

An example of its usage would be install a package.

```json
  "Install": [
    "apt-get install package1 -y"
  ]
```

### Check/Reload commands

It is recommended to use some sort of pre-check/validation/test option for your first reload command for a particular config file.
Something like `nginx -t` or `nft -f /etc/nftables.conf -c` ensures that the syntax of the file you are pushing is valid before enabling the new config.
This also ensures that if the actual reload command (like `systemctl restart`) fails, that the system is left running the previously known-good config.

These reload commands will be grouped when identical between several files in the deployment.
This ensures that if you change multiple files that all require the same systemd service to be restarted, that the service is only restarted once.

If you want to run any commands prior to the new configuration being written, use the `Checks` JSON array in the metadata header.
Check commands that fail for a group of files sharing the same reload commands will cause the reloads to NOT run (although all files which have checks that do not fail will be written to remote host)
Check commands are not grouped together and will run multiple times even if identical between multiple files.

Example metadata JSON:

```json
  "Checks": [
    "nslookup required.domain.com"
  ],
  "Reload": [
    "service1 --test-configuration -c /etc/service1/conf",
    "systemctl restart service1",
    "systemctl is-active service1"
  ]
```

### BASH Auto-Completion

In order to get auto-completion of the controller's arguments, SSH hosts, and git commit hashes, add this function to your `~/.bashrc`

If your controller binary is named something else, rename both `_controller` and `controller` to your name (keeping the underscore prefix)

```bash
# Auto completion for SCMP Controller arguments
_controller() {
    local cur prev opts

    # Define all available options
    opts="--config --deploy-changes --deploy-all --deploy-failures --execute --remote-hosts --remote-files --local-files --commitid --dry-run --max-conns --modify-vault-password --new-repo --seed-repo --install --allow-deletions --disable-privilege-escalation --ignore-deployment-state --regex --log-file --disable-reloads --force --test-config --verbose --help --version --versionid"

    # Get the current word the user is typing
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Autocompletion for options
    if [[ ${cur} == -* ]]
    then
        COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
        return 0
    fi

    # Autocomplete for file URI - Bash strips out 'file:' for some reason
    if [[ "$cur" == "//"* ]]
    then
        # Expand the ~ to the full home directory path
        cur="${cur/#\~/$HOME}"

        # Remove leading '//' from the current word to autocomplete the file path
        local file_path="${cur#//}"

        # Attempt to complete the path without '//'
        completions=($(compgen -f -- "$file_path"))

        # Add '//' to the beginning of each completion result
        for i in "${!completions[@]}"; do
            # Add '//' to the beginning
            completions[$i]="//${completions[$i]}"

            # Check if the completion is a directory and add trailing slash - doesn't work if leading with ~/
            if [[ -d "${completions[$i]#//}" ]]
            then
                completions[$i]="${completions[$i]}/"  # Append trailing slash for directories
            fi
        done

        # Set the completion results (prevent addition of spaces after)
        COMPREPLY=("${completions[@]}")
        compopt -o nospace
        return 0
    fi

    # Autocomplete arguments for specific information
    case ${prev} in
        --config | --local-files | -c | -l)
            # Expand the ~ to the full home directory path
            cur="${cur/#\~/$HOME}"

            # Generate completions for both files and directories
            local completions
            completions=( $(compgen -o dirnames -f -- "$cur") )

            # Check if completions are directories or files
            COMPREPLY=()
            for item in "${completions[@]}"; do
                if [[ -d "$item" ]]; then
                    # Append a trailing slash for directories
                    COMPREPLY+=( "${item}/" )
                    compopt -o nospace
                elif [[ -f "$item" ]]; then
                    COMPREPLY+=( "${item}" )
                fi
            done
            return 0
            ;;
        --remote-hosts | --modify-vault-password | -r | -p)
            # Autocomplete hostnames from SSH config file (default: ~/.ssh/config)
            local ssh_config="${HOME}/.ssh/config"
            if [[ -f "${ssh_config}" ]]
            then
                # Extract hostnames from the SSH config file
                COMPREPLY=( $(awk '/^Host / {print $2}' "${ssh_config}" | grep -i "^${cur}") )
            fi
            return 0
            ;;
        --commitid)
            # Autocomplete commit ids from git log if in a repository
            if [[ -d ".git" ]]
            then
                # Extract commit hash from log
                local commit_hashes
                commit_hashes=$(git log --pretty=format:"%H" -n 10)
                COMPREPLY=( $(compgen -W "${commit_hashes}" -- ${cur}) )
            fi
            return 0
            ;;
        --max-conns)
            COMPREPLY=( $(compgen -W "1 5 10 15" -- ${cur}) )
            return 0
            ;;
        --verbose)
            COMPREPLY=( $(compgen -W "0 1 2 3 4 5" -- ${cur}) )
            return 0
            ;;
    esac

    # No specific completion, show the general options
    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
    return 0
}
# Register completion for SCMP Controller
complete -F _controller controller
```

### Commit Automatic Rollback

When the controller is called with its `--git-commit` argument, there is a feature that will automatically roll back the commit when encountering an error.
During the processing of a commit, any error before the controller connects to remote hosts will result the HEAD being moved to the previous commit.

This is intentional to ensure that the HEAD commit is the most accurate representation of what configurations are currently deployed in the network.

One thing to note however, the controller does not perform garbage collection on the git repository.
Therefore it is recommended to run the following commands on a regular schedule or occasionally to reduce disk space usage (if the default git garbage collection schedule is too slow for you).

```bash
git reflog expire --expire-unreachable=now --all
git gc --prune=now
```

OR if the repository was created using the controllers option `--new-repo`, then the garbage collection options should be set in the local repository config (As of controller v1.6.0).
