// Package for prompting remote (over http) users and awaiting response
package prompt

import (
	"context"
	"fmt"
	"scmp/internal/global"
	"scmp/web/datastore"
	"strings"
	"time"

	"github.com/google/uuid"
)

// For embedding in api responses
type PromptReq struct {
	AssociatedDataID string `json:"associatedDataID"`
	PromptID         string `json:"promptID"`
	Title            string `json:"title"`
	Details          string `json:"details"`
	PromptType       string `json:"type"`
}

// Internal to datastore
type prompt struct {
	id         string
	title      string
	details    string
	promptType string
	answerChan chan []byte
}

func WaitForInput(ctx context.Context, title string, details string) (answer string, err error) {
	promptType := "regular"

	username := global.AssertFromContext[string](ctx, "username", global.UserKey, "string")
	ctxID := global.AssertFromContext[string](ctx, "ctxID", global.IDKey, "string")

	userResponse, err := request(username, ctxID, title, details, promptType)
	if err != nil {
		return
	}

	answer = strings.TrimSpace(string(userResponse))
	return
}

func WaitForSecret(ctx context.Context, title string, details string) (answer []byte, err error) {
	promptType := "secret"

	username := global.AssertFromContext[string](ctx, "username", global.UserKey, "string")
	ctxID := global.AssertFromContext[string](ctx, "ctxID", global.IDKey, "string")

	answer, err = request(username, ctxID, title, details, promptType)
	if err != nil {
		return
	}
	return
}

// Creates an internal request for user supplied information
// Blocks until user responds
func request(username string, promptID string, title string, details string, promptType string) (answer []byte, err error) {
	var promptQueue []prompt

	// Check if existing prompts are saved
	data, err := datastore.Get(username, promptID)
	if err == nil {
		existingPrompts := global.AssertType[[]prompt](data, "existingPrompts", "[]prompt")
		// Bring out any existing prompts
		promptQueue = existingPrompts
	}
	err = nil

	// Create a new prompt queue entry with a new channel
	newQueueEntry := prompt{
		id:         uuid.New().String(),
		title:      title,
		details:    details,
		promptType: promptType,
		answerChan: make(chan []byte),
	}

	promptQueue = append(promptQueue, newQueueEntry)

	// Store the new entry in the user's queue
	datastore.Put(username, promptID, promptQueue)

	// Block until the user answers or timeout occurs
	select {
	case answer = <-newQueueEntry.answerChan:
		cleanupPrompt(username, promptID, newQueueEntry.id)
		return
	case <-time.After(900 * time.Second):
		err = fmt.Errorf("timeout: no answer received from user %s for queueID %s", username, promptID)
		close(newQueueEntry.answerChan)
		cleanupPrompt(username, promptID, newQueueEntry.id)
		return
	}
}

// Checks if user has any pending prompts
func HasPending(username string, ctxID string) (hasPrompt bool) {
	// Error exists when no data is present
	data, err := datastore.Get(username, ctxID)
	if err != nil {
		hasPrompt = false
		return
	}

	queue, ok := data.([]prompt)
	if ok && len(queue) > 0 {
		hasPrompt = true
	}
	return
}

// Retrieve users pending prompts
func GetPending(username string, ctxID string) (prompts []PromptReq, err error) {
	// Quick recheck
	if !HasPending(username, ctxID) {
		return
	}

	// Safe to ignore error, check above covers
	data, _ := datastore.Get(username, ctxID)
	promptQueue := global.AssertType[[]prompt](data, "promptQueue", "[]prompt")

	// Filter and collect the title, details, and type for pending jobs
	for _, info := range promptQueue {
		req := PromptReq{
			AssociatedDataID: ctxID,
			PromptID:         info.id,
			Title:            info.title,
			Details:          info.details,
			PromptType:       info.promptType,
		}
		prompts = append(prompts, req)
	}

	return
}

// Sends user response data back to the expecting go routine
func Answer(username, ctxID, promptID string, userResponse []byte) (err error) {
	// Retrieve current prompt queue
	data, err := datastore.Get(username, ctxID)
	if err != nil {
		return fmt.Errorf("failed to retrieve datastore for user %s: %w", username, err)
	}

	promptQueue := global.AssertType[[]prompt](data, "promptQueue", "[]prompt")

	// Look for the prompt with the matching ID
	for _, prompt := range promptQueue {
		if prompt.id == promptID {
			// Send the response safely
			select {
			case prompt.answerChan <- userResponse:
				// Response sent
			default:
				err = fmt.Errorf("prompt %s for user %s is not ready to receive answer", promptID, username)
				return
			}
			return
		}
	}

	err = fmt.Errorf("no pending prompt with ID %s for user %s", promptID, username)
	return
}

// Removes an entry from the prompt queue based on its ID
func cleanupPrompt(username, ctxID, promptID string) {
	data, err := datastore.Get(username, ctxID)
	if err != nil {
		return
	}

	promptQueue := global.AssertType[[]prompt](data, "promptQueue", "[]prompt")

	for i, p := range promptQueue {
		if p.id == promptID {
			// Remove the prompt from slice
			promptQueue = append(promptQueue[:i], promptQueue[i+1:]...)
			break
		}
	}

	if len(promptQueue) == 0 {
		// no prompts left, remove entire datastore object
		datastore.Delete(username, ctxID)
	} else {
		datastore.Put(username, ctxID, promptQueue)
	}
}
