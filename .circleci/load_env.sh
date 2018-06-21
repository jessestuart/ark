#!/bin/sh
echo 'export VERSION=$(curl -s https://api.github.com/repos/${GITHUB_REPO}/releases/latest | jq -r ".tag_name")' >> $BASH_ENV
echo 'export IMAGE_ID="${REGISTRY}/${IMAGE}:${VERSION}-${TAG}"' >> $BASH_ENV
echo 'export DIR=`pwd`' >> $BASH_ENV
echo 'export GITHUB_REPO=heptio/ark' >> $BASH_ENV
echo 'export GOPATH=/home/circleci/go' >> $BASH_ENV
echo 'export GOROOT=/usr/local/go' >> $BASH_ENV
echo 'export IMAGE=ark' >> $BASH_ENV
echo 'export REGISTRY=jessestuart' >> $BASH_ENV
echo 'export QEMU_VERSION=v2.12.0'

source $BASH_ENV
