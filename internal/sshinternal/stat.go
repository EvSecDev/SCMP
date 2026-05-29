package sshinternal

import (
	"fmt"
	"scmp/internal/str"
	"strconv"
	"strings"
)

// Parses custom format used with stat command
// Relies on the stat formatting found in global constant statCmd
func ExtractMetadataFromStat(statOutput string) (fileInfo RemoteFileInfo, err error) {
	// Index Names:
	// - 0 = name
	// - 1 = type - see global const
	// - 2 = User
	// - 3 = Group
	// - 4 = PermissionBits
	// - 5 = Size in bytes
	// - 6 = Dereferenced name if applicable, otherwise just file name in single quotes
	//[/etc/rmt],[symbolic link],[root],[root],[777],[13],['/etc/rmt' -> '/usr/sbin/rmt']
	const linkDelimiter string = "' -> '"
	const bsdLinkPrefix string = "target="

	// Trim stray newlines from input if they exist
	statOutput = strings.TrimSuffix(statOutput, "\n")

	// Separate CSV into fields
	statFields := strings.Split(statOutput, ",")
	if len(statFields) != 7 {
		// Refuse any stat that does not have the exact expected number of fields
		err = fmt.Errorf("invalid file metadata: expected 7 fields, received %d fields: received %v", len(statFields), statFields)
		return
	}

	// Extract data from each field, validating field is within bounds
	for fieldIndex, field := range statFields {
		// Ensure Prefix is present
		if !strings.HasPrefix(field, "[") {
			err = fmt.Errorf("incorrect field prefix: missing prefix character '[' in value '%s'", field)
			return
		}

		// Ensure Suffix is present
		if !strings.HasSuffix(field, "]") {
			err = fmt.Errorf("incorrect field suffix: missing suffix character ']' in value '%s'", field)
			return
		}

		// Trim prefix and suffix from field text
		statFields[fieldIndex] = strings.TrimPrefix(statFields[fieldIndex], "[")
		statFields[fieldIndex] = strings.TrimSuffix(statFields[fieldIndex], "]")
	}

	// Handle linux symlink field parsing if present
	if strings.Contains(statFields[6], linkDelimiter) {
		// Split on the link point string
		dereferencedFields := strings.Split(statFields[6], linkDelimiter)

		// Ensure string was properly separated
		if len(dereferencedFields) != 2 {
			err = fmt.Errorf("could not identify dereferenced link target name from value '%s'", statFields[6])
			return
		}

		// Trim single quotes from stat output
		dereferencedFields[1] = strings.TrimPrefix(dereferencedFields[1], "'")
		dereferencedFields[1] = strings.TrimSuffix(dereferencedFields[1], "'")

		// Save back into array
		statFields[6] = dereferencedFields[1]
	} else if strings.HasPrefix(statFields[6], bsdLinkPrefix) {
		linkTarget := strings.TrimPrefix(statFields[6], bsdLinkPrefix)

		// Not checking if anything is present, stat will put the prefix in always
		statFields[6] = linkTarget
	} else {
		// Linux stat puts file name in link field - must remove
		statFields[6] = ""
	}

	// Reject file names with newlines
	if strings.Contains(statFields[0], "\n") || strings.Contains(statFields[6], "\n") {
		err = fmt.Errorf("file names with newlines are unsupported")
		return
	}

	// Put all parsed data into structured return
	fileInfo.Name = str.RemotePath(statFields[0])
	fileInfo.FsType = strings.ToLower(statFields[1]) // BSD uses capitals, linux does not
	fileInfo.Owner = statFields[2]
	fileInfo.Group = statFields[3]
	fileInfo.LinkTarget = str.RemotePath(statFields[6])

	// Assert permission string as integer
	permissionBits, err := strconv.Atoi(statFields[4])
	if err != nil {
		err = fmt.Errorf("permission bits not a number: %w", err)
		return
	}
	fileInfo.Permissions = permissionBits

	// Assert file size string as integer
	fileSizeBytes, err := strconv.Atoi(statFields[5])
	if err != nil {
		err = fmt.Errorf("file size not a number: %w", err)
		return
	}
	fileInfo.Size = fileSizeBytes

	// Valid input to this function implies it exists
	fileInfo.Exists = true
	return
}
