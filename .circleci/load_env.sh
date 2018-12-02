#!/bin/bash
echo '
export DIR=`pwd`
export GITHUB_REPO=heptio/ark
export GOPATH=/root/go
export GOROOT=/usr/local/go
export IMAGE=ark
export IMAGE_ID="${REGISTRY}/${IMAGE}:${VERSION}-${TAG}"
export QEMU_VERSION=v3.0.0
export REGISTRY=jessestuart
export VERSION=$(curl -s https://api.github.com/repos/${GITHUB_REPO}/releases/latest | jq -r ".tag_name")
' >>$BASH_ENV

source $BASH_ENV
