#!/bin/bash

. $BASH_ENV
echo "$DOCKERHUB_PASS" | docker login -u "$DOCKERHUB_USER" --password-stdin

function push_from_args() {
  local tag=$1
  local target_image=$2
  local platforms='linux/amd64,linux/arm,linux/arm64'
  local template="$REGISTRY/$target_image:$VERSION-ARCH"
  echo "Pushing manifest with template: $template, target: $REGISTRY/$target_image:$tag"
  manifest-tool push from-args \
    --platforms $platforms \
    --template $template \
    --target "$REGISTRY/$target_image:$tag"
}

push_from_args $VERSION 'velero'
push_from_args $VERSION 'velero-restic-restore-helper'

if [ "${CIRCLE_BRANCH}" == 'master' ]; then
  push_from_args 'latest' 'velero'
  push_from_args 'latest' 'velero-restic-restore-helper'
fi
