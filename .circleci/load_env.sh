#!/bin/bash
echo 'export IMAGE_ID="${REGISTRY}/${IMAGE}:${VERSION}-${TAG}"' >>$BASH_ENV
echo 'export DIR=$(pwd)' >>$BASH_ENV
echo 'export GITHUB_REPO=heptio/velero' >>$BASH_ENV
echo 'export GOPATH=/root/go' >>$BASH_ENV
echo 'export GOROOT=/usr/local/go' >>$BASH_ENV
echo 'export IMAGE=velero' >>$BASH_ENV
echo 'export REGISTRY=jessestuart' >>$BASH_ENV
echo 'export QEMU_VERSION=v3.1.0-2' >>$BASH_ENV
echo 'export VERSION=$(curl -s https://api.github.com/repos/${GITHUB_REPO}/releases/latest | jq -r ".tag_name")' >>$BASH_ENV

# echo 'export VERSION=$(curl -s https://api.github.com/repos/${GITHUB_REPO}/releases | jq -r "sort_by(.tag_name)[-1].tag_name")' >>$BASH_ENV

source $BASH_ENV
