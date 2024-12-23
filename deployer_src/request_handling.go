package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ###################################
//      REQUEST HANDLING
// ###################################

func executeCommand(channel ssh.Channel, receivedCommand string) (err error) {
	// Parse command for exe and args
	commandArray := strings.Fields(receivedCommand)
	commandBinary := commandArray[0]

	printMessage(VerbosityData, "    Preparing command %s\n", receivedCommand)

	// Prep command and args for execution
	cmd := exec.Command(commandBinary, commandArray[1:]...)
	// Init command buffers
	var stdout, stderr, channelBuff bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	printMessage(VerbosityProgress, "    Receiving stdin from client\n")

	// Get stdin from client
	_, err = io.Copy(&channelBuff, channel)
	if err != nil {
		return
	}

	// Prepare stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	defer stdin.Close()

	printMessage(VerbosityData, "    Executing command %s\n", receivedCommand)

	// Run the command
	err = cmd.Start()
	if err != nil {
		return
	}

	printMessage(VerbosityProgress, "    Writing stdin from client to command\n")

	// Write channel contents to stdin and close input
	_, err = stdin.Write(channelBuff.Bytes())
	if err != nil {
		return
	}
	stdin.Close()

	printMessage(VerbosityProgress, "    Waiting for command to finish\n")

	// Wait for command to finish
	// Errors here get sent to client, but are not applicable for this functions returned error
	err = cmd.Wait()

	printMessage(VerbosityProgress, "    Command finished\n")

	// Determine exit code to send back
	var exitCode int
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Command failed with a non-zero exit code
			exitCode = exitError.ExitCode()
			stderr.WriteString(err.Error())
		} else {
			if strings.Contains(err.Error(), "executable file not found in ") {
				exitCode = 127 // Command not found
				stderr.WriteString(err.Error())
			} else {
				exitCode = 126 // Command exists but cannot execute
				stderr.WriteString("Command exists but cannot execute\n")
			}
		}
	} else {
		exitCode = 0   // Command executed successfully
		stderr.Reset() // Ensure stderr isn't present if exit code is 0 (because sudo -S puts password prompt in stderr)
	}

	printMessage(VerbosityProgress, "    Sending stdout and stderr back to client\n")

	// Send command output back through channel
	io.Copy(channel, &stdout)
	io.Copy(channel.Stderr(), &stderr)

	printMessage(VerbosityProgress, "    Sending exit status back to client\n")

	// Send exit status back through channel
	exitStatus := make([]byte, 4)
	binary.BigEndian.PutUint32(exitStatus, uint32(exitCode))
	channel.SendRequest("exit-status", false, exitStatus)

	// Ensure err var is empty before returning
	err = nil

	return
}

// SFTP abstracted session handling
func HandleSFTP(channel ssh.Channel) (err error) {
	// Create new SFTP server for this channel
	sftpServer, err := sftp.NewServer(channel)
	if err != nil {
		return
	}
	defer sftpServer.Close()

	// Serve any commands from client
	err = sftpServer.Serve()
	if err != nil {
		return
	}
	return
}

// Use file path inside SSH request payload to run defined update program
func HandleUpdate(channel ssh.Channel, request *ssh.Request, updateTarget string) (err error) {
	// Retrieve new deployer binary path from payload of request
	updateSourceFile, err := StripPayloadHeader(request.Payload)
	if err != nil {
		err = fmt.Errorf("failed to strip request payload header: %v", err)
		return
	}

	// Send confirmation of payload receipt
	if request.WantReply {
		request.Reply(true, nil)
	}

	// Log update start
	printMessage(VerbosityStandard, "Received update request for %s, running update program\n", updateTarget)

	// Run updater program given the location of the new deployer binary
	var command string
	if updateTarget == "deployer" {
		command = UpdaterProgram + " -src " + updateSourceFile
	} else if updateTarget == "updater" {
		command = UpdaterProgram + " --update-updater -src " + updateSourceFile
	}
	err = executeCommand(channel, command)
	if err != nil {
		// return error
		err = fmt.Errorf("failed updater execution: %v", err)

		// Some errors dont get written to the channel in executeCommand function (??idk why)
		var execErr bytes.Buffer
		execErr.Write([]byte(err.Error()))
		io.Copy(channel.Stderr(), &execErr)
		return
	}

	// Update succeeded - log
	if updateTarget == "deployer" {
		printMessage(VerbosityStandard, "Stopping SCM Deployer SSH server... (update)\n")
	} else if updateTarget == "updater" {
		printMessage(VerbosityStandard, "Update of updater succeeded\n")
	}
	return
}
