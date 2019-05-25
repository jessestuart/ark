#!/bin/bash
echo '
export IMAGE_ID="${REGISTRY}/${IMAGE}:${VERSION}-${TAG}"
export DIR=$(pwd)
export GITHUB_REPO=heptio/velero
export GOPATH=/root/go
export GOROOT=/usr/local/go
export IMAGE=velero
export REGISTRY=jessestuart
export QEMU_VERSION=v4.0.0
export VERSION=$(curl -s https://api.github.com/repos/${GITHUB_REPO}/releases/latest | jq -r ".tag_name")
' >>$BASH_ENV

. $BASH_ENV
