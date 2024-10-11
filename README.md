# SCMPusher

## Description

A secure and automated configuration management terminal-based tool backed by git to centrally control and push configuration files to Linux servers.

This program is designed to assist and automate a Linux administrators job functions by centrally allowing them to edit, version control, and deploy changes to configuration files of remote Linux systems.

The controller utilizes a local git repository of a specific structure to track configuration files that should be applied to every host administered, and specific config overrides as well as host-specific configurations.
Using the GO x/crypto/ssh package, this program will SSH into the hosts defined in the configuration file and write the relevant configurations as well as handle the reloading of the associated service/program if required.
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
Below you can find the recommended setup for the remote servers, and how to configure the remote host to have the least privileges possible to fulfill the functions of this program.

This is a prototype and may have unintended consequences to managed systems as development is ongoing. Use at your own risk.
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
- Removing directories no previously in repository
- Changing owner and group of directories
- Changing permissions of directories
- Manage executables or shared objects
- Deploy without existing commandlets (ls, rm, mv, ect.)

### What it **can** do: Controller

- Deploy automatically via git post-commit hook
- Deploy manually via specifying commit hash
- Easy recovery from partial deployment failures
- One-time manual deployment to specific hosts
- Deploy all relevant files (even unchanged) to a newly created remote host 
- Fail-safe file deployment - automatic restore if any remote failure is encountered
- Concurrent SSH Connections to handle a large number of remote hosts (and option to limit concurrency)
- Support for regular SSH servers (if you don't want to use the Deployer program)
- Key-based SSH authentication (by file or ssh-agent)
- Password-based Sudo command escalation

### What it **can't** do: Controller

- SSH Password logins
- SSH 2FA (TOTP) logins
- Different SSH keys per host (Use another yaml config in a separate repository for network enclaves with different keys)
- Different Sudo passwords per host
- Multiple different SSH key algorithms across remote hosts
- Handle some special files (device, pipes, sockets, ect.)

### Controller Help Menu

```
Usage of scmcontroller:
-V 			Print Version Information
-auto-deploy 		Automatically uses latest commit and deploys configuration files from it (Requires '-c [/path/to/scmpc.yaml]')
-c [string] 		Path to the configuration file (default "scmpc.yaml")
-commitid [string] 	Commit ID (hash) of the commit to deploy configurations from (Requires '-c [/path/to/scmpc.yaml]')
-deploy-all 		Ignores changed files, and deploys all relevant files in repo to specified hosts (Requires '--remote-hosts [hostname]')
-manual-deploy 		Manually use supplied commit ID to deploy configs for repository (Requires '--commitid [hash]'
-remote-hosts [string] 	Override hosts (by name in comma separated values) which will be deployed to (Requires '--commitid [hash]')
-use-failtracker-only 	Use the fail tracker file in the given commit to manually deploy failed files (Requires '--manual-deploy'
```

### Deployer Help Menu

```
Usage of scmdeployer:
-V 		Print Version Information
-c [string] 	Path to the configuration file (default "scmpd.yaml")
-start-server	Start the Deployer SSH Server (Requires '-c [/path/to/scmpd.yaml])
```

## Setup walk-through

### Controller (local) setup

1. Create an ED25519 SSH private key (Yes, that specific algo, otherwise you'll have to change the source code - read the note at the bottom), and copy public key to desired hosts
2. Start the controller binary and the supplied template yaml configuration on your workstation or deployment source of choice with the '--newrepo' argument.
3. Follow the prompts to initialize a new repository.
4. Configure the template yaml file for all the remote Linux hosts you wish to manage, with their IP, Port, and username
5. Copy all the remote configuration files you wish to manage into the repository under the desired template or host directory, then git add and commit to test functionality.

### Remote Hosts Setup

1. Run the supplied installer shell script for your architecture
2. Answer the questions as it applies to your environment
3. If on Debian, copy supplied PAM Apparmor template to remote host
4. Add the public key for the user/Deployer to the controller's configuration file for that particular host

## NOTES

### SSH Key Algorithm limitation

Due to the way that Go's SSH package works (and SSH in general), it is not feasible for me to acquire every single algorithm an SSH server can offer and attempt to match a local key when there could potentially be hundreds or thousands of remote hosts to connect to.
For this reason, the program has been intentionally limited to supporting only a single key algorithm type for all remote hosts and the local SSH key. It is currently hard coded as ED25519. 
This means that you must use an ED25519 private key, and all remote SSH servers must support ED25519.

If this does not fit your environment, feel free to adjust the source code and build a modified binary for RSA or DSA keys. But be warned, ALL remote hosts must support the same algorithm. 

### Reason for a separate SSH server (the Deployer)

You might be asking yourself, "Why is there a custom SSH server for this program? I already have an OpenSSH server".
That is a good question, and if you don't care to read why, the controller fully supports a regular SSH server and you can ignore the Deployer program altogether.

But as for why, I wanted extra security measures around this program that can read and write to almost everything on a given system. 
So I wrote an extremely stripped down, bare-bones SSH server as I could and applied security measures to that. 
This reduces the attack surface immensely and allows for a very narrow set of privileges to be granted.

The Deployer program only supports:
  - one connection at a time
  - one authentication method (username+key)
  - one channel at a time
  - one request at a time
  - two request types, exec and sftp

But don't worry (too much), I didn't make an SSH server from scratch.
I am using the library x/crypto/ssh, and while I am creating my own implementation, it is so featureless that the risk of an high impact RCE is unlikely (in my own code, not the source library).
The secondary affect of such a bare-bones server program also means I can tailor the installation to be extremely narrow.
Not only can the entire server be run as a nologin non-root system user, it can also be wrapped in a very restrictive apparmor confinement, since the server only fulfills this one function (configuration deployment)
With a standard SSH server, being more versatile and feature-rich, would need a comparatively more unrestricted profile (Not to mention that the SSH parent process is run as root).

For example, the apparmor profile:
  - confines the server process to its own profile.
  - confines every sudo process to its own profile.
  - confines every command run by sudo to a dedicated profile, and restricts the allowed scope of deployment.
  - confines every reload command to a dedicated profile, and restricts the allowed scope of reloads.

