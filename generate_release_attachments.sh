#!/bin/bash

# Ensure script is run only in bash
if [ -z "$BASH_VERSION" ]
then
        echo "This script must be run in BASH."
        exit 1
fi

echo "========================================"
read -p " Press enter to continue"
echo "========================================"

reporoot=$(pwd)

# Build controller
cd controller_src/
./build_and_package.sh
mv controller_package_* ~/Downloads/
./build_and_package.sh build
mv controller_* ~/Downloads/

cd $reporoot

# Build deployer
cd deployer_src/
./build_and_package.sh
mv deployer_installer_* ~/Downloads/
./build_and_package.sh sigbuild
mv deployer_* ~/Downloads/

# Build updater
cd updater_src/
./build-static.sh build
mv updater_* ~/Downloads/

cd $reporoot

# Generate release notes from git commit message

headcommit=$(git log -1)

comment_Added=$(echo -e "$headcommit" | grep "   Added" | sed 's/^[ \t]*Added/ -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')
comment_Changed=$(echo -e "$headcommit" | grep "   Changed" | sed 's/^[ \t]*Changed/ -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')
comment_Removed=$(echo -e "$headcommit" | grep "   Removed" | sed 's/^[ \t]*Removed/ -/g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')
comment_Fixed=$(echo -e "$headcommit" | grep "   Fixed" | sed 's/^[ \t]*Fixed/ -/g' | sed 's/bug where //g' | sed 's/^\([^a-zA-Z]*\)\([a-zA-Z]\)/\1\U\2/')

echo "DOUBLE CHECK GRAMMAR BEFORE USING"
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

ls -l ~/Downloads/
