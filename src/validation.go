// controller
package main

import (
	"bytes"
)

func isHex40(input string) (matches bool) {
	if len(input) != 40 {
		matches = false
		return
	}
	for _, char := range input {
		if !((char >= '0' && char <= '9') ||
			(char >= 'a' && char <= 'f') ||
			(char >= 'A' && char <= 'F')) {
			matches = false
			return
		}
	}
	matches = true
	return
}

func hasHex64Prefix(input string) (matches bool, matchedText string) {
	if len(input) < 64 {
		matches = false
		return
	}
	for i := range 64 {
		char := input[i]
		if !((char >= '0' && char <= '9') ||
			(char >= 'a' && char <= 'f') ||
			(char >= 'A' && char <= 'F')) {
			matches = false
			return
		}
	}
	matches = true
	matchedText = input[:64]
	return
}

// isText checks if a string is likely plain text or binary data based on the first 500 bytes
func isText(inputBytes *[]byte) (isPlainText bool) {
	// Allow 30% non-printable in input
	const maximumNonPrintablePercentage float64 = 30

	totalCharacters := len(*inputBytes)
	if totalCharacters > 500 {
		totalCharacters = 500
	}

	// Empty files can be treated as plain text (Avoid divide by 0)
	if totalCharacters == 0 {
		isPlainText = true
		return
	}

	// PDF files have a start that is plain text, identify PDF header to reject it as plain text
	if len(*inputBytes) > 9 {
		PDFHeaderBytes := []byte{37, 80, 68, 70, 45, 49, 46, 52, 10}
		headerComparison := bytes.Compare((*inputBytes)[:9], PDFHeaderBytes)
		if headerComparison == 0 {
			isPlainText = false
			return
		}
	}

	// Count the number of characters outside the ASCII printable range (32-126) - skipping DEL
	var nonPrintableCount int
	for i := range totalCharacters {
		b := (*inputBytes)[i]
		if b < 32 || b > 126 {
			nonPrintableCount++
		}
	}

	// Get percentage of non printable characters found
	nonPrintablePercentage := (float64(nonPrintableCount) / float64(totalCharacters)) * 100
	printMessage(verbosityData, "  Data is %.2f%% non-printable ASCII characters (max: %g%%)\n", nonPrintablePercentage, maximumNonPrintablePercentage)

	// Determine if input is text or binary
	if nonPrintablePercentage < maximumNonPrintablePercentage {
		isPlainText = true
	} else {
		isPlainText = false
	}
	return
}
