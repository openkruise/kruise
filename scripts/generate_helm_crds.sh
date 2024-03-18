#!/bin/bash

# Check if yq command is installed, otherwise install it
if ! command -v yq &> /dev/null; then
    echo "yq command not found. You need install yq first."
    exit 0
fi

rm -rf bin/templates
mkdir bin/templates

# Get the current directory
current_dir=$(pwd)
echo $current_dir
# Create a temporary directory
temp_dir=$(mktemp -d)
echo $temp_dir

kustomize build config/crd > $temp_dir/crds.yaml
cd $temp_dir
awk 'BEGIN {RS="\n---\n"} {print > ("output-" NR ".yaml")}' crds.yaml
rm -f crds.yaml

# Process each file in the directory
for file in *; do
    # Parse the YAML file and extract the value of the desired fields
    group=$(yq eval '.spec.group' $file)
    plural=$(yq eval '.spec.names.plural' $file)

    # Remove leading and trailing whitespace from the field values
    group=$(echo $group | sed -e 's/^ *//' -e 's/ *$//')
    plural=$(echo $plural | sed -e 's/^ *//' -e 's/ *$//')

    sed -i '.bak' '1i\
{{- if .Values.crds.managed }}\
 \
---
    ' $file
    rm -f $file.bak
    echo "{{- end }}" >> $file
    # Rename the file using the extracted field values
    mv $file "$current_dir/bin/templates/${group}_${plural}.yaml"
done

# Clean up the temporary directory
cd ..
rm -rf $temp_dir
