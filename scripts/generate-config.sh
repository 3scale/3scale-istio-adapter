#!/usr/bin/env bash

# This script provides a way to generate the required files for an out-of-process adapter
# Requires Istio source code to be on $GOPATH and will generate based on the locally checked
# out version of Istio
# Runs a diff of generated files against current configuration

script_dir=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
root_dir=$(dirname "${script_dir}")

istio_src="${GOPATH-$HOME/go}/src/istio.io/istio"
mixer_src="${istio_src}/mixer"
target_dir="${mixer_src}/adapter/3scaleAdapter"


# Test the existence of provided directory
# Args: (path to directory)
verify_dir_exists() {
    if [[ ! -d "${1}" ]];
    then
        echo "Required directory not found in ${1}"
        exit 1
    fi
}

# Copies the required files in place for go generate command
copy_adapter_files() {
    verify_dir_exists "${root_dir}/config"
    cp -r pkg/threescale "${target_dir}"
    cp -r config/config.proto "${target_dir}/config"
}

verify_dir_exists "${istio_src}"
mkdir -p "${target_dir}/config"
copy_adapter_files
cd "${target_dir}" && go generate ./...

compare_extensions=(
  yaml
  go
  proto_descriptor
)

for i in "${compare_extensions[@]}" ; do
  for file in "${target_dir}/config/"*.${i} ; do
    diff -b "$file" "config/${file##*/}"
  done
done


