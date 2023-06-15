#!/usr/bin/env bash

# Converts the folder name that the component documentation file
# is stored in into the integration slug of the component.
componentTypeFromFolderName() {
    if [[ "$1" = "builders" ]]; then
        echo "builder"
    elif [[ "$1" = "provisioners" ]]; then
        echo "provisioner"
    elif [[ "$1" = "post-processors" ]]; then
        echo "post-processor"
    elif [[ "$1" = "datasources" ]]; then
        echo "data-source"
    else
        echo ""
    fi
}

# $1: The content to adjust links
# $2: The organization of the integration
rewriteLinks() {
  local result="$1"
  local organization="$2"

  urlSegment="([^/]+)"
  urlAnchor="(#[^/]+)"

  # Rewrite Component Index Page links to the Integration root page.
  #
  #                    (\1)     (\2)      (\3)
  # /packer/plugins/datasources/amazon#anchor-tag-->
  # /packer/integrations/hashicorp/amazon#anchor-tag
  local find="\(\/packer\/plugins\/$urlSegment\/$urlSegment$urlAnchor?\)"
  local replace="\(\/packer\/integrations\/$organization\/\2\3\)"
  result="$(echo "$result" | sed -E "s/$find/$replace/g")"


  # Rewrite Component links to the Integration component page
  #
  #                    (\1)      (\2)       (\3)       (\4)
  # /packer/plugins/datasources/amazon/parameterstore#anchor-tag -->
  # /packer/integrations/{organization}/amazon/latest/components/datasources/parameterstore
  local find="\(\/packer\/plugins\/$urlSegment\/$urlSegment\/$urlSegment$urlAnchor?\)"
  local replace="\(\/packer\/integrations\/$organization\/\2\/latest\/components\/\1\/\3\4\)"
  result="$(echo "$result" | sed -E "s/$find/$replace/g")"

  # Rewrite the Component URL segment from the Packer Plugin format
  # to the Integrations format
  result="$(echo "$result" \
      | sed "s/\/datasources\//\/data-source\//g" \
      | sed "s/\/builders\//\/builder\//g" \
      | sed "s/\/post-processors\//\/post-processor\//g" \
      | sed "s/\/provisioners\//\/provisioner\//g" \
  )"

  echo "$result"
}

# $1: Docs Dir
# $2: Web Docs Dir
# $3: Component File
# $4: The org of the integration
processComponentFile() {
    local docsDir="$1"
    local webDocsDir="$2"
    local componentFile="$3"

    local escapedDocsDir="$(echo "$docsDir" | sed 's/\//\\\//g' | sed 's/\./\\\./g')"
    local componentTypeAndSlug="$(echo "$componentFile" | sed "s/$escapedDocsDir\///g" | sed 's/\.mdx//g')"

    # Parse out the Component Slug & Component Type
    local componentSlug="$(echo "$componentTypeAndSlug" | cut -d'/' -f 2)"
    local componentType="$(componentTypeFromFolderName "$(echo "$componentTypeAndSlug" | cut -d'/' -f 1)")"
    if [[ "$componentType" = "" ]]; then
        echo "Failed to process '$componentFile', unexpected folder name."
        echo "Documentation for components must be stored in one of:"
        echo "builders, provisioners, post-processors, datasources"
        exit 1
    fi


    # Calculate the location of where this file will ultimately go
    local webDocsFolder="$webDocsDir/components/$componentType/$componentSlug"
    mkdir -p "$webDocsFolder"
    local webDocsFile="$webDocsFolder/README.md"
    local webDocsFileTmp="$webDocsFolder/README.md.tmp"

    # Copy over the file to its webDocsFile location
    cp "$componentFile" "$webDocsFile"

    # Remove the Header
    local lastMetadataLine="$(grep -n -m 2 '^\-\-\-' "$componentFile" | tail -n1 | cut -d':' -f1)"
    cat "$webDocsFile" | tail -n +"$(($lastMetadataLine+2))"  > "$webDocsFileTmp"
    mv "$webDocsFileTmp" "$webDocsFile"

    # Remove the top H1, as this will be added automatically on the web
    cat "$webDocsFile" | tail -n +3 > "$webDocsFileTmp"
    mv "$webDocsFileTmp" "$webDocsFile"

    # Rewrite Links
    rewriteLinks "$(cat "$webDocsFile")" "$4" > "$webDocsFileTmp"
    mv "$webDocsFileTmp" "$webDocsFile"
}

# Compiles the Packer SDC compiled docs folder down
# to a integrations-compliant folder (web docs)
#
# $1: The directory of the plugin
# $2: The directory of the SDC compiled docs files
# $3: The output directory to place the web-docs files
# $4: The org of the integration
compileWebDocs() {
  local docsDir="$1/$2"
  local webDocsDir="$1/$3"

  echo "Compiling MDX docs in '$2' to Markdown in '$3'..."
  # Create the web-docs directory if it hasn't already been created
  mkdir -p "$webDocsDir"

  # Copy the README over
  cp "$docsDir/README.md" "$webDocsDir/README.md"

  # Process all MDX component files (exclude index files, which are unsupported)
  for file in $(find "$docsDir" | grep "$docsDir/.*/.*\.mdx" | grep --invert-match "index.mdx"); do
    processComponentFile "$docsDir" "$webDocsDir" "$file" "$4"
  done
}

compileWebDocs "$1" "$2" "$3" "$4"
