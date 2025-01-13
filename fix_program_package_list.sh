#!/bin/bash

# first argument is which program to get imports for
program=$1

# current dir
repoRoot=$(pwd)

# Set search directory and entrance go file depending on user choice
if [[ $program == controller ]]
then
	searchDir="$repoRoot/controller_src"
	mainFile=$(grep -il "func main() {" $searchDir/*.go | egrep -v "testing")
else
	# Only allow certain args
	echo "No program specified - must choose 'controller'"
	exit 1
fi

# Hold cumulative (duplicated) imports from all go source files
allImports=""

# Loop all go source files
for gosrcfile in $(find "$searchDir/" -maxdepth 1 -iname *.go)
do
	# Get space delimited single line list of imported package names (no quotes) for this go file
	allImports+=$(cat $gosrcfile | awk '/import \(/,/\)/' | egrep -v "import \(|\)|^\n$" | sed -e 's/"//g' -e 's/\s//g' | tr '\n' ' ' | sed 's/  / /g')
done

# Put space delimited list of all the imports into an array
IFS=' ' read -r -a pkgarr <<< "$allImports"

# Create associative array for deduping
declare -A packages

# Add each import package to the associative array to delete dups
for pkg in "${pkgarr[@]}"
do
	packages["$pkg"]=1
done

# Convert back to regular array
allPackages=("${!packages[@]}")

# search line in each program that contains package import list for version print
packagePrintLine='fmt.Print("Packages: '

# Format package list into go print line
newPackagePrintLine=$'\t\tfmt.Print("Packages: '"${allPackages[@]}"'\\n")'

# Write new package line into go source file that has main function
sed -i "/$packagePrintLine/c\\$newPackagePrintLine" $mainFile
