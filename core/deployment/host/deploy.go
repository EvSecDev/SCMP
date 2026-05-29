package host

import (
	"context"
	"fmt"
	"scmp/core/deployment"
	"scmp/core/deployment/predeploy"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"

	"golang.org/x/crypto/ssh"
)

// SSH's into a remote host to deploy files and run reload commands
func (deployer *Deployer) Deploy(ctx context.Context, deployFiles *deployment.HostFiles) {
	// Signal routine is done after return
	defer deployer.allHostWG.Done()

	deployer.connLimiter <- struct{}{}
	defer func() { <-deployer.connLimiter }()

	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logctx.LogStdFatal(ctx,
				"Controller panic during deployment to host '%s': %v\n",
				deployer.host.EndpointName, fatalError)
		}
	}()

	ctx = logctx.AppendCtxTag(ctx, string(deployer.host.EndpointName))

	// Save meta info for this host in a structure to easily pass around required pieces
	deployer.state.Name = deployer.host.EndpointName
	deployer.state.Password = deployer.host.Password

	err := predeploy.RunPreDeploymentCommands(ctx, deployer.metrics, deployer.state.Name, deployFiles)
	if err != nil {
		err = fmt.Errorf("failed to run pre-deployment commands: %w", err)
		deployer.metrics.AddAllDeployFiles(deployer.state.Name, deployFiles)
		deployer.metrics.AddHostFailure(deployer.state.Name, err)
		return
	}

	select {
	case <-ctx.Done():
		err = fmt.Errorf("immediate stop requested before beginning deployment to host %s", deployer.state.Name)
		deployer.metrics.AddAllDeployFiles(deployer.state.Name, deployFiles)
		deployer.metrics.AddHostFailure(deployer.state.Name, err)
		return
	default:
	}

	// Connect to the SSH server
	var proxyClient *ssh.Client
	deployer.state.SSHClient, proxyClient, err = sshinternal.ConnectToSSH(ctx, deployer.host, deployer.proxy)
	if err != nil {
		err = fmt.Errorf("failed connect to SSH server: %w", err)
		deployer.metrics.AddAllDeployFiles(deployer.state.Name, deployFiles)
		deployer.metrics.AddHostFailure(deployer.state.Name, err)
		return
	}
	defer func() {
		if proxyClient != nil {
			lerr := proxyClient.Close()
			if err == nil && lerr != nil {
				err = fmt.Errorf("proxy close: %w", lerr)
			}
		}
		lerr := deployer.state.SSHClient.Close()
		if err == nil && lerr != nil {
			err = fmt.Errorf("client close: %w", lerr)
		}
	}()

	// Pre-deployment checks
	err = RemoteDeploymentPreparation(ctx, &deployer.state)
	if err != nil {
		err = fmt.Errorf("remote system preparation failed: %w", err)
		deployer.metrics.AddAllDeployFiles(deployer.state.Name, deployFiles)
		deployer.metrics.AddHostFailure(deployer.state.Name, err)
		return
	}
	defer CleanupRemote(ctx, deployer.state)

	// Deploy files concurrently
	for _, independentDeploymentList := range deployFiles.Groups {
		group := newGroupDeployer(deployer)
		deployer.deployWG.Add(1)

		if deployer.maxConcurrentDeploys > 1 {
			go group.deploy(ctx, independentDeploymentList, deployFiles.GlobalFiles)
		} else {
			// Max conns of 1 disables using go routine
			group.deploy(ctx, independentDeploymentList, deployFiles.GlobalFiles)

			// File groups are considered fully independent, errors do not stop further groups from starting deployment
			// dependencies/reloads/reload groups are the mechanism to use to halt further file deployments
		}
	}
	deployer.deployWG.Wait()
}
