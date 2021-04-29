#!/bin/bash

set -e

###
# Update PATH Variables
###

export GOPATH=$HOME/gopath/
export ISTIO=$GOPATH/src/istio.io/
mkdir -p $GOPATH/bin
export PATH=$PATH:$GOPATH/bin/


###
# Install & Setup Golang dep
###

curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
