#!/bin/sh

. $BASH_ENV
echo "$DOCKERHUB_PASS" | docker login -u "$DOCKERHUB_USER" --password-stdin
manifest-tool push from-args \
  --platforms linux/amd64,linux/arm,linux/arm64 \
  --template "$REGISTRY/$IMAGE:$VERSION-ARCH" \
  --target "$REGISTRY/$IMAGE:$VERSION"
if [ "${CIRCLE_BRANCH}" == 'master' ]; then
  manifest-tool push from-args \
    --platforms linux/amd64,linux/arm,linux/arm64 \
    --template "$REGISTRY/$IMAGE:$VERSION-ARCH" \
    --target "$REGISTRY/$IMAGE:latest"
fi
