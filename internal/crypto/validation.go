package crypto

import "unicode"

func IsValidUsername(username string) (valid bool) {
	// Limit length
	if len(username) < 3 || len(username) > 32 {
		return
	}

	for index, char := range username {
		// must start with a letter
		if index == 0 && !unicode.IsLetter(char) {
			return
		}
		// only allow letters, digits, and _
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '_' {
			return
		}
	}

	valid = true
	return
}

func IsValidPassword(password string) (valid bool) {
	length := len(password)
	if length < 8 || length > 128 {
		return
	}

	// Minimum complexity
	var hasLetter, hasDigit, hasUpper, hasSpecial bool

	for _, char := range password {
		switch {
		case unicode.IsLetter(char):
			hasLetter = true
			if unicode.IsUpper(char) {
				hasUpper = true
			}
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if hasLetter && hasDigit && hasUpper && hasSpecial {
		valid = true
	}
	return
}
