#!/bin/bash

function update_readme {
	local helpMenu READMEmdFileName srcStartDelimiter readmeStartDelimiter readmeHelpMenuEndDelimiter helpMenu menuSectionStartLineNumber helpMenuDelimiter helpMenuStartLine helpMenuEndLine
	helpMenu=$1
	srcStartDelimiter=$2
	readmeStartDelimiter=$3
	readmeHelpMenuEndDelimiter='```'
	READMEmdFileName="README.md"

	echo "[*] Copying program help menu from source file to README..."

	if [[ -z $helpMenu ]]
	then
		echo "[-] Warning: no help menu retrieved from compiled binary, not updating readme"
		return 1
	fi

	# Line number for start of md section
	menuSectionStartLineNumber=$(grep -n "$readmeStartDelimiter" "$READMEmdFileName" | cut -d":" -f1)
	helpMenuDelimiter=$readmeHelpMenuEndDelimiter

	# Line number for start of code block
	helpMenuStartLine=$(awk -v startLine="$menuSectionStartLineNumber" -v delimiter="$helpMenuDelimiter" '
	  NR > startLine && $0 ~ delimiter { print NR; exit }
	' "$READMEmdFileName")

	# Line number for end of code block
	helpMenuEndLine=$(awk -v startLine="$helpMenuStartLine" -v delimiter="$helpMenuDelimiter" '
          NR > startLine && $0 ~ delimiter { print NR; exit }
        ' "$READMEmdFileName")

	# Replace existing code block with new one
	awk -v start="$helpMenuStartLine" -v end="$helpMenuEndLine" -v replacement="$helpMenu" '
	    NR < start { print }                # Print lines before the start range
	    NR == start {                       # Print the start line and replacement text
	        print
	        print replacement
	    }
	    NR > start && NR < end { next }     # Skip lines between start and end
	    NR == end { print }                 # Print the end line
	    NR > end { print }                  # Print lines after the end range
	' "$READMEmdFileName" > .t && mv .t "$READMEmdFileName"
}
