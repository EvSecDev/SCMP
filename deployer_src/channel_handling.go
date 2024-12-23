package main

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ###################################
//      CHANNEL HANDLING
// ###################################

// Define a handler for SSH connections
func handleChannel(newChannel ssh.NewChannel) {
	// Recover from panic
	defer func() {
		if r := recover(); r != nil {
			logError("Panic while processing client channel", fmt.Errorf("%v", r), false)
		}
	}()

	printMessage(VerbosityProgress, " Accepting new channel\n")

	// Accept the channel
	channel, requests, err := newChannel.Accept()
	if err != nil {
		logError("SSH channel error", fmt.Errorf("could not accept channel: %v", err), false)
		return
	}
	defer channel.Close()

	printMessage(VerbosityProgress, " Accepted channel\n")

	// Loop client requests - Only allow SFTP or Exec
	for request := range requests {
		printMessage(VerbosityProgress, "  Processing request\n")

		switch strings.ToLower(request.Type) {
		case "exec":
			printMessage(VerbosityProgress, "   Received exec request\n")

			command, err := StripPayloadHeader(request.Payload)
			if err != nil {
				logError("SSH request error", fmt.Errorf("exec: failed to strip request payload header: %v", err), false)
				break
			}
			if request.WantReply {
				request.Reply(true, nil)
			}
			err = executeCommand(channel, command)
			if err != nil {
				logError("SSH request error", fmt.Errorf("failed command execution: %v", err), false)
				break
			}
		case "subsystem":
			printMessage(VerbosityProgress, "   Received subsystem request\n")

			subsystem, err := StripPayloadHeader(request.Payload)
			if err != nil {
				logError("SSH request error", fmt.Errorf("subsystem: failed to strip request payload header: %v", err), false)
				break
			}
			if subsystem != "sftp" {
				request.Reply(false, nil)
				logError("SSH request error", fmt.Errorf("received unauthorized subsystem %s", subsystem), false)
				break
			}

			printMessage(VerbosityProgress, "   Received subsystem sftp request\n")

			if request.WantReply {
				request.Reply(true, nil)
			}
			// Handle SFTP
			err = HandleSFTP(channel)
			if err != nil {
				logError("SSH request error", fmt.Errorf("failed sftp: %v", err), false)
				break
			}
		case "update":
			printMessage(VerbosityProgress, "   Received update request\n")

			// Run Update for deployer
			err = HandleUpdate(channel, request, "deployer")
			if err != nil {
				logError("SSH request error: update", err, false)
				break
			}
		case "updateupdater":
			printMessage(VerbosityProgress, "   Received update updater request\n")

			// Run Update for updater
			err = HandleUpdate(channel, request, "updater")
			if err != nil {
				logError("SSH request error: update", err, false)
				break
			}
		case "getupdaterversion":
			printMessage(VerbosityProgress, "   Received update version check request\n")

			if request.WantReply {
				request.Reply(true, nil)
			}

			// Get version of updater executable and return through channel
			command := UpdaterProgram + " --versionid"
			err = executeCommand(channel, command)
			if err != nil {
				logError("SSH request error", fmt.Errorf("failed command execution: %v", err), false)
				break
			}
		default:
			logError("SSH request error", fmt.Errorf("unauthorized request type %s received", request.Type), false)
			request.Reply(false, nil) // Reject unknown requests
		}
		channel.Close()
	}
}
