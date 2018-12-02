#!/bin/bash

WORKDIR="$GOPATH/src/github.com/$GITHUB_REPO"
mkdir -p "$WORKDIR" && cd "$WORKDIR" || exit 1

# ============
# <qemu-support>
if [ $GOARCH == 'amd64' ]; then
  touch qemu-amd64-static
else
  curl -sL "https://github.com/multiarch/qemu-user-static/releases/download/${QEMU_VERSION}/qemu-${QEMU_ARCH}-static.tar.gz" | tar xz
  docker run --rm --privileged multiarch/qemu-user-static:register
fi
# </qemu-support>
# ============

# Replace the repo's Dockerfile with our own.
cp -f "$DIR/Dockerfile" .
docker build -t $IMAGE_ID \
  --build-arg target=$TARGET \
  --build-arg goarch=$GOARCH \
  --build-arg image=$GITHUB_REPO .

# Login to Docker Hub.
docker login -u $DOCKERHUB_USER -p $DOCKERHUB_PASS
# Push push push
docker push ${IMAGE_ID}
if [ $CIRCLE_BRANCH == 'master' ]; then
  docker tag "${IMAGE_ID}" "${REGISTRY}/${IMAGE}:latest-${TAG}"
  docker push "${REGISTRY}/${IMAGE}:latest-${TAG}"
fi
