# SCMPusher

## Description

A secure and automated configuration management terminal-based tool backed by git to centrally control and push configuration files to Linux servers.

This program is designed to assist and automate a Linux administrators job functions by centrally allowing them to edit, version control, and deploy changes to configuration files of remote Linux systems.
This program is NOT intended as a configuration management system (like Terraform), but rather a CLI tool to replace the manual process of SSH'ing into many remote servers to manage configuration files.

There are three parts to this tool (two of which are optional), the controller, deployer, and updater.
 - The controller is the client that runs on your workstation and pushes configuration files to remote servers.
 - The deployer is the (optional) server that runs on the remote servers that will process the configuration files received from the controller.
 - The updater is the (optional) helper program that also runs on the remote servers that will validate new deployer executables that the controller sends to the deployer server.

The controller utilizes a local git repository of a specific structure to track configuration files that should be applied to every host administered, and specific config overrides as well as host-specific configurations.
Using the Go x/crypto/ssh package, this program will SSH into the hosts defined in the configuration file and write the relevant configurations as well as handle the reloading of the associated service/program if required.
  The deployment method is currently only SSH by key authentication using password sudo for remote commands (password login authentication is currently not supported).
 - In automatic deployment mode, every time you commit a change to the git repository, the program will be called with the 'post-commit' hook and will deploy the changed files to their designated remote hosts.
 - In manual deployment mode, you can choose a specific commit ID from your repository and deploy the changed files in that specific commit to their designated remote hosts.
 - In manual deployment mode with failtracker, the program will read the last failure json (if present) and extract the commitid, hosts, and files that failed and attempt to redeploy.
 - In deploy all mode, with a comma separated list of hosts, you can deploy every relevant file in the repo to the chosen hosts for a given commit (usually, the head commit). 

Although this program does need permissions on remote systems for writing system-wide configuration files and potentially restarting services, it does NOT need to SSH as root.
It is recommended to use sudo with a password with either a standard SSH server or the custom Deployer SSH server with some or all of these below security precautions.
  - Only allowing sudo for ls, rm, mv, cp, ln, rmdir, mkdir, chown, chmod, sha256sum, and your reload commands.
  - Using network level host IP authentication (such as IPsec AH)
  - Using the supplied apparmor profile for the controller.
  - Regular encrypted backups of git repository
  - If using the custom Deployer SSH server, also consider:
    - Running as a non-login, non-root, system user.
    - Using the supplied apparmor profile for the supplied custom Deployer SSH server (with modifications for your reload commands).
    - Using the supplied updater program to update the Deployer executable from the controller (This verifies the new Deployer binary by digital signature prior to update)
Below you can find the recommended setup for the remote servers, and how to configure the remote host to have the least privileges possible to fulfill the functions of this program.

This is a work-in-progress and may have unintended consequences to managed systems as development is ongoing. Use at your own risk.
If you like what this program can do or want to expand functionality yourself, feel free to submit a pull request or fork.

## Capabilities Overview

### What it **can** do: Deployer (Remote Host Actions)

- Create new files
- Create new directories
- Modify existing files content
- Modify owner and group of files
- Modify permissions of files
- Removing 'managed' files
- Removing empty 'managed' directories
- Run a linear series of commands to enable/reload/start services associated with files

### What it **can't** do: Deployer (Remote Host Actions)

- Removing files not previously in repository
- Removing directories not previously in repository
- Changing owner and group of directories
- Changing permissions of directories
- Manage executables or shared objects
- Deploy without existing commandlets on remote system (ls, rm, mv, ect.)

### What it **can** do: Controller

- Deploy automatically via git post-commit hook
- Deploy manually via specifying commit hash
- Easy recovery from partial deployment failures
- One-time manual deployment to specific hosts and/or specific files
- Fail-safe file deployment - automatic restore of previous file version if any remote failure is encountered
- Deploy all (or a subset of) relevant files (even unchanged) to a newly created remote host
- Concurrent SSH Connections to handle a large number of remote hosts (and option to limit concurrency)
- Support for regular SSH servers (if you don't want to use the Deployer program)
- Key-based SSH authentication (by file or ssh-agent, per host or all hosts)
- Password-based Sudo command escalation
- Create new repositories
- Collect configurations from existing systems to bootstrap the local repository

### What it **can't** do: Controller

- SSH Password logins
- SSH 2FA (TOTP) logins
- Handle some special files (device, pipes, sockets, ect.)

### Controller Help Menu

```
Usage: controller [OPTIONS]...

Examples:
    controller --config </etc/scmpc.yaml> --manual-deploy --commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7> [--remote-hosts <www,proxy,db01>] [--local-files <www/etc/hosts,proxy/etc/fstab>]
    controller --config </etc/scmpc.yaml> --manual-deploy --use-failtracker-only
    controller --config </etc/scmpc.yaml> --deploy-all --remote-hosts <www,proxy,db01> [--commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7>]
    controller --config </etc/scmpc.yaml> --deployer-versions [--remote-hosts <www,proxy,db01>]
    controller --config </etc/scmpc.yaml> --deployer-update-file <~/Downloads/deployer> [--remote-hosts <www,proxy,db01>]
    controller --new-repo /opt/repo1:main
    controller --config </etc/scmpc.yaml> --seed-repo [--remote-hosts <www,proxy,db01>]

Options:
    -c, --config </path/to/yaml>                    Path to the configuration file [default: scmpc.yaml]
    -a, --auto-deploy                               Use latest commit for deployment, normally used by git post-commit hook
    -m, --manual-deploy                             Use specified commit ID for deployment (Requires '--commitid')
    -d, --deploy-all                                Deploy all files in specified commit to specific hosts (Requires '--remote-hosts')
    -r, --remote-hosts <host1,host2,...>            Override hosts for deployment
    -l, --local-files <file1,file2,...>             Override files for deployment
    -C, --commitid <hash>                           Commit ID (hash) of the commit to deploy configurations from
    -f, --use-failtracker-only                      If previous deployment failed, use the failtracker to retry (Requires '--manual-deploy', but not '--commitid')
    -t, --test-config                               Test controller configuration syntax and configuration option validity
    -T, --dry-run                                   Prints available information and runs through all actions before initiating outbound connections
    -q, --deployer-versions                         Query remote host deployer executable versions and print to stdout
    -u, --deployer-update-file </path/to/exe>       Upload and update deployer executable with supplied signed ELF file
    -n, --new-repo </path/to/repo>:<branch>         Create a new repository at the given path with the given initial branch name
    -s, --seed-repo                                 Retrieve existing files from remote hosts to seed the local repository (Requires user interaction and '--remote-hosts')
    -g, --disable-git-hook                          Disables the automatic deployment git post-commit hook for the current repository
    -G, --enable-git-hook                           Enables the automatic deployment git post-commit hook for the current repository
    -h, --help                                      Show this help menu
    -V, --version                                   Show version and packages
    -v, --versionid                                 Show only version number
```

### Deployer Help Menu

```
Usage: scmdeployer [OPTIONS]...

Options:
    -c, --config </path/to/yaml>       Path to the configuration file [default: scmpd.yaml]
    -s, --start-server                 Start the Deployer SSH Server
    -t, --test-config                  Test deployer configuration syntax validity
    -T, --dry-run                      Runs through all actions and checks for error before starting server
    -h, --help                         Show this help menu
    -V, --version                      Show version and packages
    -v, --versionid                    Show only version number
```

## Setup walk-through

### Remote Hosts Setup

**If using the Deployer SSH server:**
1. Copy the install archive to the installation host
2. Extract the archive with tar
3. Run the installation shell script, answer the prompts
4. Done! Proceed to controller installation

**If not using the Deployer SSH server:**
1. Create a user that can log into SSH and use Sudo
2. Modify `/etc/sudoers` to allow your new user to run Sudo commands with a password
  - Optionally, restrict the commands your new user can run to the following:
    - ls, rm, cp, ln, rmdir, mkdir, chown, chmod, sha256sum, and any reload commands you need (systemctl, sysctl, ect.)
3. Done! Proceed to controller installation

### Controller (local) setup

1. Create an SSH private key
2. Copy the public key of your new SSH key to the desired hosts
3. Start the installer script and follow the prompts
4. Configure the template yaml file for all the remote Linux hosts you wish to manage, with their IP, Port, and username (and indicate true if you don't want a specific host to use the templates directory)
5. Copy all the remote configuration files you wish to manage into the repository under the desired template or host directory
6. Then, git add and commit to test functionality.

## Bootstrapping the Repository

So, what if you already have servers with potentially hundreds of configuration files spread throughout the system?
Well, fear not, for there is a SSH client built into the controller as an easier method of transferring and formatting new files by hand.
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
You can type as many or as little options as you wish, they will all be added.
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

### Warning about the known_hosts file

Beware, using your existing known_hosts file from a standard SSH client is not recommended.
In some implementations, multiple SSH clients do not play well writing hosts to a single known_hosts.
In practice, this will look like the controller is continuously prompting you to trust keys for remote hosts you have already trusted.

It is recommended to store the known_hosts file (path specified in the controller YAML configuration file) inside the root of repository.

### Reason for a separate SSH server (the Deployer)

You might be asking yourself, "Why is there a custom SSH server for this program? I already have an SSH server".
That is a good question, and if you don't care to read why, the controller fully supports a regular SSH server and you can ignore the Deployer program altogether.

But as for why, I wanted extra security measures around this program, since it supposed to read and write to almost everything on a given system.
So I wrote an extremely stripped down, bare-bones SSH server as I could and applied security measures to that. 
This reduces the attack surface immensely and allows for a very narrow set of privileges to be granted.

The Deployer program only supports:
  - one connection at a time
  - one authentication method (username+key)
  - one channel at a time
  - one request at a time
  - three request types, exec, update, and sftp

But don't worry (too much), I didn't make an SSH server from scratch.
I am using the library x/crypto/ssh, and while I am creating my own implementation, it is so featureless that the risk of an high impact RCE is unlikely (in my own code, not the source library).
The secondary affect of such a bare-bones server program also means I can tailor the installation to be extremely narrow.
Not only can the entire server be run as a nologin non-root system user, it can also be wrapped in a very restrictive apparmor confinement, since the server only fulfills this one function (configuration deployment).
comparatively, a standard SSH server (being more versatile and feature-rich) would need a more open apparmor profile (Not to mention that the SSH parent process is run as root).

For example, the apparmor profile:
  - confines the server process to its own profile.
  - confines every sudo process to its own profile.
  - confines every command run by sudo to a dedicated profile, and restricts the allowed scope of deployment.
  - confines every reload command to a dedicated profile, and restricts the allowed scope of reloads.

You might also question how you would update all these Deployer executables since it doesn't use your systemd package manager. 
Well, good news! The controller can push updated Deployer executables for you. 
When a new Deployer executable is released, simply download and use the controller and whichever remote hosts (or all of them) to update.
The controller will transfer the new file over, and the old Deployer will launch the dedicated updater program. 
This updater program will verify the embedded digital signature inside the new Deployer executable. 
Then (based on it's parent process) will kill the Deployer process, move the new executable in place, and the new Deployer will start automatically (Because of the systemd auto-restart feature).

### How the updater works

Design requirements for the Deployer updater system:
- Ensure that the Deployer process cannot write to its own executable file or configuration file
- Ensure that the Deployer process cannot alter received updated executables
- Ensure that the updater program itself is robust and rarely needs updating itself

With those requirements, the Updater operates in the following manner:
1. User initiates an updater from the controller by passing the file path on the controller machine and specifying which remote hosts to update.
2. Controller SFTP transfers the new Deployer binary to all the remote hosts (using the `/tmp` buffer file).
3. Controller issues SSH request type `update` to each remote host along with the Sudo password via standard in.
  - Note: `update` is a custom request type not present in normal SSH servers and will fail at this stage if you accidentally try to update a host without Deployer installed
4. The Deployer process on the remote host recognizes the custom request type and uses the updater executable file path in its own configuration file.
5. The Deployer process launches the updater process as a child process running as its own user and passes the Sudo password via standard in.
6. The Updater process will retrieve the embedded digital signature in the new updated Deployer executable and use the embedded public key to verify the new file.
7. The Updater process will assume its parent PID is the Deployer process and kill that PID.
8. The Updater process will copy the Deployer buffer file from `/tmp` to the executable location of the old Deployer process keeping the destination permissions.
9. The Updater process will remote the buffer file from `/tmp` and then exit.
10. Systemd will auto-restart the Deployer process (now updated) after 60 seconds.

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
