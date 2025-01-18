# SCMPusher

## Description

A secure and automated configuration management terminal-based tool backed by git to centrally control and push configuration files to Linux servers through SSH.

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
  - Only allowing your user sudo access for ls, rm, mv, cp, ln, rmdir, mkdir, chown, chmod, sha256sum, and your reload commands.
  - Using network level host IP authentication (such as IPsec AH)
  - Using the supplied apparmor profile for the controller.
  - Regular encrypted backups of git repository
Below you can find the recommended setup for the remote servers, and how to configure the remote host to have the least privileges possible to fulfill the functions of this program.

If you like what this program can do or want to expand functionality yourself, feel free to submit a pull request or fork.

## Capabilities Overview

### What it **can** do: Remote Host Actions

- Create new files
- Create new directories
- Modify existing files content
- Modify owner and group of files
- Modify permissions of files
- Removing 'managed' files
- Removing empty 'managed' directories
- Run a linear series of commands to enable/reload/start services associated with files

### What it **can't** do: Remote Host Actions

- Removing files not previously in repository
- Removing directories not previously in repository
- Changing owner and group of directories
- Changing permissions of directories
- Manage executables or shared objects
- Deploy without existing commandlets on remote system (ls, rm, mv, ect.)

### What it **can** do: Local Actions

- Deploy automatically via git post-commit hook
- Deploy manually via specifying commit hash
- Easy recovery from partial deployment failures
- Deployment test run using single host (use `--max-conns 1 -r HOST`)
- One-time manual deployment to specific hosts and/or specific files
- Fail-safe file deployment - automatic restore of previous file version if any remote failure is encountered
- Deploy all (or a subset of) relevant files (even unchanged) to a newly created remote host
- Group hosts together to allow single universal configuration files to deploy to all or a subset of remote hosts
- Concurrent SSH Connections to handle a large number of remote hosts (and option to limit concurrency)
- Key-based SSH authentication (by file or ssh-agent, per host or all hosts)
- Password-based Sudo command escalation
- Create new repositories
- Collect configurations from existing systems to bootstrap the local repository

### What it **can't** do: Local Actions

- SSH Password logins
- SSH 2FA (TOTP) logins
- Use SSH Control Sockets
- Use any form of client forwarding (tunnels, x11, agents)
- Handle some special files (device, pipes, sockets, ect.)

### Controller Help Menu

```
Examples:
    controller --config <~/.ssh/config> --deploy-changes [--commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7>] [--remote-hosts <www,proxy,db01>] [--local-files <www/etc/hosts,proxy/etc/fstab>]
    controller --config <~/.ssh/config> --deploy-failures
    controller --config <~/.ssh/config> --deploy-all [--remote-hosts <www,proxy,db01>] [--commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7>]
    controller --new-repo /opt/repo1:main
    controller --config <~/.ssh/config> --seed-repo [--remote-hosts <www,proxy,db01>]

Options:
    -c, --config </path/to/ssh/config>         Path to the configuration file [default: ~/.ssh/config]
    -d, --deploy-changes                       Deploy changed files in the specified commit [commit default: head]
    -a, --deploy-all                           Deploy all files in specified commit [commit default: head]
    -f, --deploy-failures                      Deploy failed files/hosts using failtracker file from last failed deployment
    -r, --remote-hosts <host1,host2,...>       Override hosts for deployment
    -l, --local-files <file1,file2,...>        Override files for deployment (Must be relative file paths from root of the repository)
    -C, --commitid <hash>                      Commit ID (hash) of the commit to deploy configurations from
    -T, --dry-run                              Prints available information and runs through all actions without initiating outbound connections
    -m, --max-conns <15>                       Maximum simultaneous outbound SSH connections [default: 10] (Use 1 to disable deployment concurrency)
    -p, --modify-vault-password <host>         Create/Change/Delete a hosts password in the vault (will create the vault if it doesn't exist)
    -n, --new-repo </path/to/repo>:<branch>    Create a new repository at the given path with the given initial branch name
    -s, --seed-repo                            Retrieve existing files from remote hosts to seed the local repository (Requires user interaction and '--remote-hosts')
    -g, --disable-git-hook                     Disables the automatic deployment git post-commit hook for the current repository
    -G, --enable-git-hook                      Enables the automatic deployment git post-commit hook for the current repository
    -t, --test-config                          Test controller configuration syntax and configuration option validity
    -v, --verbosity <0...5>                    Increase details and frequency of progress messages (Higher number is more verbose) [default: 1]
    -h, --help                                 Show this help menu
    -V, --version                              Show version and packages
        --versionid                            Show only version number

Documentation: <https://github.com/EvSecDev/SCMPusher>
```

## Setup and Configuration

1. Create an SSH private key
    - `ssh-keygen -t ed25519 -N '' -C scmp/controller -f controller_ssh`
2. Start the installer script and follow the prompts
    - `./controller_installer*sh`
3. Configure the YAML configuration file for all the remote Linux hosts you wish to manage (see comments in YAML for what the fields mean)
4. Done! Proceed to remote preparation

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

This feature requires that you have installed controller and configured the controller's yaml configuration file with the hosts you want to manage.
It also requires that the remote host is setup as described in the controller's yaml (port is open, user is allowed, ect.)

The interface you will be using for this feature is extremely barebones. It looks like this:
```
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

`  hostname:/# Type your selections: c4`

If you wanted to select files `vmlinuz`, `initrd.img.old`, `initrd.img`, and then exit you'd type this and press enter:

`  hostname:/# Type your selections: 23 7 6 !`

If you were in `/etc/` and wanted to move up one directory, you'd type this and press enter:

`  hostname:/# Type your selections: c0`

The shortcuts will be listed below every directory so you won't need to remember them.
You can type as many or as little options as you wish in any order, they will all be added.
Selected files will be saved before changing directories, so you can navigate the whole remote host file system saving files you want as you go.

Once you have selected all your files and typed `!`, you will be asked (file by file) if the config requires reload commands, and if so, you can provided them one per line.
The controller will then take all the files and write them to their respective host directories in the local repository copying the remote host file path.

The structure of the local repository is supposed to be a replica of the remote server filesystem, to facilitate editing and organizing files as you normally would on the remote system.
```
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
  -> Host3
    -> root
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
You can specify the directory name and the hosts that should use the directory in the main YAML config.

### File transfers

File transfers for this program are done using SCP and are limited to 90 seconds per file. 
Something to keep in mind, your end to end bandwidth for a deployment will determine how large of a file can be transferred in that time.

### Reload commands

It is recommended to use some sort of pre-check/validation/test option for your first reload command for a particular config file.
Something like `nginx -t` or `nft -f /etc/nftables.conf -c` ensures that the syntax of the file you are pushing is valid before enabling the new config.
This also ensures that if the actual reload command (like `systemctl restart`) fails, that the system is left running the previously known-good config.

### Commit Automatic Rollback

When the controller is running in automatic mode, there is a feature that will automatically roll back the commit when encountering an error.
During the processing of a commit, any error before the controller connects to remote hosts will result the HEAD being moved to the previous commit.

This is intentional to ensure that the HEAD commit is the most accurate representation of what configurations are currently deployed in the network.

One thing to note however, the controller does not perform garbage collection on the git repository.
Therefore it is recommended to run the following commands on a regular schedule or occasionally to reduce disk space usage (if the default git garbage collection schedule is too slow for you).
```
git reflog expire --expire-unreachable=now --all
git gc --prune=now
```

OR if the repository was created using the controllers option `--new-repo`, then the garbage collection options should be set in the local repository config (As of controller v1.6.0).
