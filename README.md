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

- In deploy diff mode, you can choose a specific commit ID (or specify none and use the latest commit) from your repository and deploy the changed files in that specific commit to their designated remote hosts.
- In deploy failures mode, the program will read the last failure json (if present) and extract the commitid, hosts, and files that failed and attempt to redeploy.
- In deploy all mode, with a comma separated list of hosts, you can deploy every relevant file in the repo to the chosen hosts for a given commit (usually, the head commit).

Although this program does need permissions on remote systems for writing system-wide configuration files and potentially restarting services, it does NOT need to SSH as root.
In general, it is recommended to use some or all of these below security precautions.

- Sudo access that requires a password.
- Only allowing your user sudo access for the standard commands (listed below in dependencies section) and your reload commands.
- Using network level host IP authentication (such as IPsec AH).
- Using the supplied apparmor profile for the controller.
- Regular encrypted backups of the git repository to a different machine.

Below you can find the recommended setup for the remote servers, and how to configure the remote host to have the least privileges possible to fulfill the functions of this program.

If you like what this program can do or want to expand functionality yourself, feel free to submit a pull request or fork.

## Capabilities Overview

### What it can do

- Deployments
  - Deploy changed configurations based on commit difference or manually via specifying a commit hash
  - Deploy all (or a subset of) tracked files by commit (default is most recent)
  - Deploy individual/lists/groups of files to individual/lists/groups of hosts
  - Deployment test run using single host (use `--max-conns 1 -r HOST`)
  - Concurrent file deployment per host (use `--max-deploy-threads`) (note: requires server support for high numbers)
  - Exclude hosts from deployments (use config option `DeploymentState offline` under a host)
  - Ad-hoc override host exclusion from deployments (use `--ignore-deployment-state`)
  - Run a linear series of commands prior to any deployment actions per file/directory (part of file JSON metadata header)
  - Run a linear series of commands to enable/reload/start services associated with files/directories (part of file JSON metadata header)
    - Option to temporarily disable globally for a deployment
  - Run a linear series of commands to install services associated with files/directories (part of file JSON metadata header)
  - Easy retry of deployment failures
  - Fail-safe file deployment - automatic restore of previous file version and service reload if any remote failure is encountered during initial reload
  - Rollback of bad configurations (succeeded reload but non-functional service) using `deploy all -C <previous commit id>`
- File/Directory Management
  - Create/modify files/file content and directories
  - Modify permissions, owner, and group of files and directories
  - Removing 'managed' files and directories
  - Creating/modifying symbolic links
  - Group files together to apply to multiple hosts
  - Options to ignore specific directories in the repository
  - Track binary/artifact files (executables, images, videos, documents)
  - Support standard ASCII naming
- Host Management
  - Use standard SSH client config to manage endpoints
  - Ability to mark individual hosts as offline to prevent deployments to that host
  - Apply file groups to distribute single file version to all or a subset of all hosts
- SSH
  - Key-based authentication (by file or ssh-agent, per host or all hosts)
  - SSH Proxy connections (Bastions, Jump hosts, ect.)
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
  - Handle special files (links, device, pipes, sockets, ect.)
- SSH
  - 2FA (TOTP) logins
  - Use Control Sockets
  - Use some forms of client forwarding (tunnels, x11, agents)

### Dependencies

- Remote Host Requirements:
  - OpenSSH Server (other servers are untested)
  - Commands: `ls, stat, rm, mv, cp, ln, rmdir, mkdir, chown, chmod, sha256sum, uname`
- Local Host Requirements:
  - Unix file paths

### Controller Help Menu

```bash
Usage: ./controller [subcommand] [arguments]...

Secure Configuration Management Program (SCMP)
  Deploy configuration files from a git repository to Linux servers via SSH
  Deploy ad-hoc commands and scripts to Linux servers via SSH

  Subcommands:
    deploy
    exec
    git
    install
    scp
    secrets
    seed
    version

  Options:
      --allow-deletions  Permits deletions of files/entries
      --force            Do not exit/abort on failures
      --log-file         Write events to log file (using --verbose level)
      --with-summary     Generate JSON summary of actions
  -c, --config           Path to the configuration file [default: ~/.ssh/config]
  -T, --dry-run          Conducts non-mutating actions (no remote actions)
  -v, --verbosity        Increase detailed progress messages (Higher is more verbose) <0...5> [default: 1]
  -w, --wet-run          Conducts non-mutating actions (including remote actions)

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
    - `controller install --repository-path /path/to/you/new/repo --repository-branch-name main`
    - 3a) **Optional**: If you want a sample configuration file, run this command
      - `controller install --default-config`
    - 3b) **Optional**: If you want to install the AppArmor profile, run this command
      - `sudo controller install --apparmor-profile`
    - 3c) **Optional**: If you want bash auto-completion for the controller arguments, see the snippet in the Notes section to add to your `~/.bashrc`
4. Configure the SSH configuration file for all the remote Linux hosts you wish to manage (see comments in config for what the fields mean)
5. Done! Proceed to remote preparation

### Remote Preparation

1. Create a user that can log into SSH and use Sudo
   - `useradd --create-home --user-group deployer`
   - `passwd deployer`
2. Add your SSH public key to the users home directory `authorized_keys` file
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

`controller seed --remote-host <host>`

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
         Select File    Change Dir ^v  Recursive  Exit
        [ # # ## ### ]  [ c0 ] [ c# ]    [ #r ]   [ ! ]
hostname:/# Type your selections: _
```

If you wanted to change directories to `/etc/`, you'd type this and press enter:

`hostname:/# Type your selections: c4`

If you wanted to select files `vmlinuz`, `initrd.img.old`, `initrd.img`, and then exit you'd type this and press enter:

`hostname:/# Type your selections: 23 7 6 !`

If you were in `/etc/` and wanted to move up one directory, you'd type this and press enter:

`hostname:/# Type your selections: c0`

If you were in `/` and wanted to recursively download all files in directory `/opt`, you'd type this and press enter:

`hostname:/# Type your selections: 13r`

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
If a particular host should never use the UniversalConf configs, then the config option `ignoreUniversalConfs` should be set to true under that particular host in the main config.

UniversalGroups is a set of directories that will only apply to a subset of hosts.
The functionality is identical to the UniversalConfs directory, but will only apply to hosts that are apart of the group.

You can specify the available universal directories in the SSH config with the global option `GroupDirs`.
You can specify the per-host universal directories in the SSH config with the host option `GroupTags`.

### Directory Management

The version control and deployment of directory and directory metadata is split in two.
The existence of a directory/directory structure will imply the creation of the same structure on the remote host.
Removal of managed directories in the repository will only remove the remote directory if the remote directory is empty.

Metadata of directories is handled through a special JSON file that lives directly under the directory it applies to.
The file name is static and will always need to be `.directory_metadata_information.json`

The JSON is the same as the metadata header in files:

```json
{
  "FileOwnerGroup": "www-data:www-data",
  "FilePermissions": 755
}
```

This metadata file is automatically created during seeding if the directory permissions differ from the default (`root:root` `rwxr-xr-x`)
This feature is not meant to be used everywhere. When new directories are created, the default will be used.
This metadata file should only be used where custom permissions are absolutely required.

### File transfers

File transfers for this program are done using SCP.
Something to keep in mind, your end to end bandwidth for a deployment will determine how large of a file can be transferred in that time.

To do bulk file transfers there is the `scp` subcommand.
It utilizes similar options as the OpenSSH SCP program.

File uploads for this argument are limited to one-to-many or one-to-one.

#### Examples one-to-one

`controller scp Local/path/to/file1 host1,host2:/path/to/file1`

```text
Local/path/to/file1
  -> host1,host2 /path/to/file1
```

`controller scp Local/path/to/file1,Local/path/to/file2 host1,host2:/path/to/file1,/path/to/file2`

```text
Local/path/to/file1
  -> host1,host2 /path/to/file2

Local/path/to/file2
  -> host1,host2 /path/to/file2
```

#### Examples one-to-many

`controller scp Local/path/to/file1 host1,host2:/path/to/file1,/path/to/file2`

```text
Local/path/to/file1
  -> host1,host2 /path/to/file1
  -> host1,host2 /path/to/file2
```

### Maximum Deployment Threads

This option describes the maximum concurrent deployment of file(s) for a given host, but is not as straight forward as one might assume.

On OpenSSH servers, there is a fairly significant delay (mostly due to network latency) between when a client closes a channel and when the server actually closes it.

For LAN configurations, it is generally safe to have `--max-deploy-threads` set to the same value of the server's `maxsessions`.

For internet hosts, the max threads will vary on the physical distance away from the controller (and thus the network latency).
Generally, for high latency hosts, error-free deployment (on the server's side) is achieved when `--max-deploy-threads` set to half of the server's `maxsessions`.

The default for this program is currently set to half of OpenSSH default `maxsessions` of 10.

The controller does include an internal retry and backoff timer so in most cases, even when `--max-deploy-threads` is set to `maxsessions`, there should be no server-side errors.

However, it is not unusual to see the following log from the SSH server:
`sshd-session[pid]: error: no more sessions`

This log should indicate that you should either increase `maxsessions` on the server, or decrease `--max-deploy-threads`.

### Dry/Wet Test Runs

Two options are present for testing deployments prior to actually performing actions.

`Dry-run` is available to test all pre-deployment actions, such as organizing files and loading content.
This will not connect to any remote host.
It's purpose to allow you to validate that the current commit is valid locally (commit rollbacks are still enabled)

`Wet-run` is available to test all pre-deployment and some deployment actions.
This will connect to remote hosts and perform setup actions and checks but will not deploy or reload anything.
Note: Check commands are still run in full in this mode.
It's purpose is to allow you to validate what would most likely happen during an actual deployment without performing mutating actions.

### Validate File Metadata Header

Here is a bash one-liner to quickly validate metadata headers before deployments if you are manually creating the JSONs

```bash
FILE="path/to/your/file"
cat $FILE | sed -n '/#|^^^|#/,/#|^^^|#/ { /#|^^^|#/b; /#|^^^|#/b; p }' | jq .
```

### Artifact Files (External Git Content)

Binary files and other non-text files (artifacts) are not great at being tracked by git.
Due to this, there is a workaround to "version control" binary files while still being tied into SCMP deployments.

This feature requires three things:

- The file in the SCMP git repository needs an extra field in the metadata header JSON: `ExternalContentLocation`:
  - Example: `"ExternalContentLocation":"file:///absolute/path/to/actual/binary/file"`
- The file in the SCMP git repository can be named whatever it needs to be, but requires `.remote-artifact` file extension.
  - The extension is just for local identification, the extension is removed prior to deployment.
- Use the built-in controller git add argument `controller git add <glob>`
  - This is how the artifact files are tracked by git.

Any content past the metadata header in the `.remote-artifact` file is used to store the hash of the artifact file content.
This hash is updated before every commit to ensure updates remotely are tracked in git without tracking the binary's actual content

You might be asking, how does the git repository know when your binary file has changed?
Since the binary file is not tracked directly in git, any `.remote-artifact` files will be flagged for inspection when using controller's `git add` argument.
The controller will follow the file path you give in the `ExternalContentLocation` and hash the current artifact file.
Once the artifact pointer file is flagged as changed by git, the normal deployment process takes place (with the caveat that content loading is done using the `ExternalContentLocation`)

Due to this system, binary files do take up extra processing power and memory space since changes are tracked at runtime.

Only `file://` (local) URIs are supported for the `ExternalContentLocation` field currently.

### Command Macros (Internal Variables/Actions)

Certain macros are supported in the JSON metadata header command strings.
These macros are replaced with known values or trigger special actions during pre-deployment file processing.

Special actions can be used inspect or modify file content before files are transferred remotely.
This allows dynamic file generation/modification without hard coding values in the repository.

Notes:

- Macro names are case-sensitive.
- Macros inside of double quotes will throw a JSON formatting error
- Special actions for stdout must be at the end of the command string

Example of expansion given input of `Server01/etc/nginx/nginx.conf`:

```text
{@FILEPATH}      -> /etc/nginx/nginx.conf
{@FILEDIR}       -> /etc/nginx
{@FILENAME}      -> nginx.conf
{@REPOBASEDIR}   -> Server01
```

Example of special actions:

Checking contents on the fly (file contents are written to standard in for the command):

```json
  "PreDeploy": [
    "/path/to/custom_script.sh <<<{@LOCALFILEDATA}"
  ]
```

Appending result of the command to the file contents:

```json
  "PreDeploy": [
    "/path/to/custom_script.sh >>{@REMOTEFILEDATA}"
  ]
```

Overwriting file contents with the command result:

```json
  "PreDeploy": [
    "/path/to/custom_script.sh >{@REMOTEFILEDATA}"
  ]
```

Reading tge file contents and then overwriting them:

```json
  "PreDeploy": [
    "/path/to/custom_script.sh <<<{@LOCALFILEDATA} >{@REMOTEFILEDATA}"
  ]
```

### Pre-Deployment Commands

In some cases it may be desired to run commands locally for a given config prior to deployment.

Commands are run immediately prior to connecting to the remote host.
Any actual local failures (missing command binary/permission issues) will result in the entire host being marked as failed.

The metadata header field looks like:

```json
  "PreDeploy": [
    "/path/to/local_check_Script.sh"
    "/path/to/syntax_checker.sh {@REPOBASEDIR} {@FILEPATH}"
  ]
```

If the script exits with anything other than exit code 0, the file (and associated files) will not be deployed.

For special actions and macros see above section.

### Inter-file Dependency

Frequently, there is a need to deploy files in a certain order.
In order to accomplish this, within a given file, you can define which files (by the local relative repository path) that it relies upon.
This ensures that during deployment, the order in which files are deployed and reloaded is controllable.

This feature does not work with files not tracked in the repository (if you require a certain remote untracked file or system state, please use `Checks` commands)

Example of metadata header:

```json
  "Dependencies": [
    "host1/etc/resolv.conf",
    "host1/etc/apt/sources.list"
  ]
```

### Symbolic Links

This program intentionally ignores OS-level symbolic links in order to decouple the file/directory management from the local filesystem.
This also frees the use of symbolic/hard links to be used within the repository itself without duplicating the link functionality on the remote system.

In order to manage symbolic links on the remote system, a dedicated metadata header field is used.

```json
  "SymbolicLinkTarget": "Host1/etc/service/file.conf"
```

The presence of this key indicates that the local file is actually a link.
The contents of the file are ignored.
The ownership/permissions are ignored.

The use of installation/checks/reload/dependency fields are still valid.

### Install commands

Commands in this metadata JSON array are run only by using the controller deploy argument `--install`.
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

If any of the reload commands fail, controller will restore the previous file version and run the reload commands again to ensure the service is properly rolled back.

These reload commands will be grouped when identical between several files in the deployment.
This ensures that if you change multiple files that all require the same systemd service to be restarted, that the service is only restarted once.

If you want to run any commands prior to the new configuration being written, use the `Checks` JSON array in the metadata header.
Check commands that fail for a group of files sharing the same reload commands will cause the reloads to NOT run (although all files which have checks that do not fail will be written to remote host)
Check commands are not grouped together and will run multiple times even if identical between multiple files.

#### Named Reload Groups

You can easily group files together so that reloads are run only after all relevant service files have been written, even if the reload commands differ.
Using the `ReloadGroup` JSON key, you can specify any arbitrary string and the controller will ensure files with the identical group string are deployed and reloaded together.
In order to ensure the correct sequence of reloads, utilize the Inter-File dependency feature listed above.

In addition, the program will automatically attempt to group any files that have an identical set of commands into named groups even if that file did not explicitly include a named group.

Example metadata JSON:

```json
  "Checks": [
    "nslookup required.domain.com"
  ],
  "Reload": [
    "service1 --test-configuration -c /etc/service1/conf",
    "systemctl restart service1",
    "systemctl is-active service1"
  ],
  "ReloadGroup": "Service 1 Config Files"
```

### BASH Auto-Completion

In order to get auto-completion of the controller's arguments, SSH hosts, and git commit hashes, add this function to your `~/.bashrc`

If your controller binary is named something else, rename both `_controller` and `controller` to your name (keeping the underscore prefix)

```bash
# Auto completion for SCMP Controller arguments
_controller() {
  local cur prev words cword
  _init_completion || return
  # Top-level subcommands
  local subcommands="deploy exec git install scp secrets seed version"
  # Global options
  local global_opts="
    --allow-deletions
    --force
    --log-file
    --with-summary
    -c --config
    -T --dry-run
    -v --verbosity
    -w --wet-run
"
  # Subcommand-specific options
  local deploy_subcommands="all diff failures"
  local deploy_opts="
    --disable-privilege-escalation
    --disable-reloads
    --execution-timeout
    --ignore-deployment-state
    --install
    --regex
    -C --commitid
    -l --local-files
    -m --max-conns
    -r --remote-hosts
    -t --test-config
    -u --run-as-user
"
  local exec_opts="--regex -r --remote-hosts -R --remote-file --disable-privilege-escalation -m --max-conns -u --run-as-user --execution-timeout"
  local git_subcommands="add commit status"
  local git_opts="-m --message"
  local install_opts="--apparmor-profile --default-config --repository-branch-name --repository-path"
  local scp_opts="-c --config"
  local secrets_opts="-p --modify-vault-password"
  local seed_opts="--regex -r --remote-hosts -R --remote-files"
  local version_opts="-v"
  # Handle custom completions
  case "${prev}" in
    --config|-c|--local-files|-l|--remote-files|-R)
        compopt -o filenames
        COMPREPLY=( $(compgen -f -- "$cur") )
        return 0
        ;;
    --remote-hosts|-r|--modify-vault-password|-p)
        local ssh_config="${HOME}/.ssh/config"
        if [[ -f "$ssh_config" ]]
        then
            COMPREPLY=( $(awk '/^Host / {print $2}' "$ssh_config" | grep -i "^$cur") )
        fi
        return 0
        ;;
    --commitid|-C)
        if [[ -d ".git" ]]
        then
            COMPREPLY=( $(git log --pretty=format:"%H" -n 20 | grep -i "^$cur") )
        fi
        return 0
        ;;
    --max-conns|-m)
        COMPREPLY=( $(compgen -W "1 5 10 15 20 50" -- "$cur") )
        return 0
        ;;
    --verbosity|-v|--verbose)
        COMPREPLY=( $(compgen -W "0 1 2 3 4 5" -- "$cur") )
        return 0
        ;;
  esac
  # Determine subcommand context
  case "${COMP_WORDS[1]}" in
    ""|-* )
        COMPREPLY=( $(compgen -W "${subcommands} ${global_opts}" -- "$cur") )
        ;;
    deploy )
        if [[ "${COMP_WORDS[2]}" =~ ^(-|$) ]]
        then
            COMPREPLY=( $(compgen -W "${deploy_subcommands} ${global_opts} ${deploy_opts}" -- "$cur") )
        else
            COMPREPLY=( $(compgen -W "${deploy_opts} ${global_opts}" -- "$cur") )
        fi
        ;;
    exec )
        COMPREPLY=( $(compgen -W "${exec_opts} ${global_opts}" -- "$cur") )
        ;;
    git )
        if [[ "${COMP_WORDS[2]}" =~ ^(-|$) ]]
        then
            COMPREPLY=( $(compgen -W "${git_subcommands} ${global_opts} ${git_opts}" -- "$cur") )
        else
            COMPREPLY=( $(compgen -W "${git_opts} ${global_opts}" -- "$cur") )
        fi
        ;;
    install )
        COMPREPLY=( $(compgen -W "${install_opts} ${global_opts}" -- "$cur") )
        ;;
    scp )
        COMPREPLY=( $(compgen -W "${scp_opts} ${global_opts}" -- "$cur") )
        ;;
    secrets )
        COMPREPLY=( $(compgen -W "${secrets_opts} ${global_opts}" -- "$cur") )
        ;;
    seed )
        COMPREPLY=( $(compgen -W "${seed_opts} ${global_opts}" -- "$cur") )
        ;;
    version )
        COMPREPLY=( $(compgen -W "${version_opts} ${global_opts}" -- "$cur") )
        ;;
    * )
        COMPREPLY=( $(compgen -W "${subcommands} ${global_opts}" -- "$cur") )
        ;;
  esac
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

OR if the repository was created using the controllers option `install --repository-path`, then the garbage collection options should be set in the local repository config (As of controller v1.6.0).
