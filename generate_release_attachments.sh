#!/bin/bash
set -e

# Ensure script is run only in bash
if [ -z "$BASH_VERSION" ]
then
        echo "This script must be run in BASH."
        exit 1
fi

echo "========================================"
read -p " Press enter to continue"
echo "========================================"

# Build controller
./build.sh -b controller -f
mv controller_v* ~/Downloads/

# Build controller package
./build.sh -b controllerpkg -f
mv controller_install* ~/Downloads/

# Build deployer
./build.sh -b deployer -f
mv deployer_v* ~/Downloads/

# Build deployer package
./build.sh -b deployerpkg -f
mv deployer_package* ~/Downloads/

# Build updater
./build.sh -b updater -f
mv updater_v* ~/Downloads/

# Generate release notes from git commit message

headcommit=$(git log -1)

comment_Added=$(echo -e "$headcommit" | grep "   Added" | sed 's/^[ \t]*Added/ -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')
comment_Changed=$(echo -e "$headcommit" | grep "   Changed" | sed 's/^[ \t]*Changed/ -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')
comment_Removed=$(echo -e "$headcommit" | grep "   Removed" | sed 's/^[ \t]*Removed/ -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')
comment_Fixed=$(echo -e "$headcommit" | grep "   Fixed" | sed 's/^[ \t]*Fixed/ -/g' | sed 's/bug where //g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')

echo -e "DOUBLE CHECK GRAMMAR BEFORE USING\n\n"

cat << EOF
### Added
$comment_Added

### Changed
$comment_Changed

### Removed
$comment_Removed

### Fixed
$comment_Fixed

### Instructions
 - Please refer to the README.md file for instructions

EOF

echo -e "FILE OUTPUTS\n\n"
ls -l ~/Downloads/
