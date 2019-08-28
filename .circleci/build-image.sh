#!/bin/sh

set -eu

IMAGE_ID="${REGISTRY}/${IMAGE}:${VERSION}-${TAG}"

# Login to Docker Hub.
echo $DOCKERHUB_PASS | docker login -u $DOCKERHUB_USER --password-stdin

WORKING_DIR="$GOPATH/src/github.com/${GITHUB_REPO}"
mkdir -p $WORKING_DIR && cd $WORKING_DIR

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

# =======================================================
# Main velero image.
# =======================================================

# Replace the repo's Dockerfile with our own.
cp -f $DIR/Dockerfile .
echo "Building image with ID: $IMAGE_ID"
docker build -t ${IMAGE_ID} \
  --build-arg target=$TARGET \
  --build-arg goarch=$GOARCH .

# Push push push
docker push ${IMAGE_ID}
if [ $CIRCLE_BRANCH == 'master' ]; then
  docker tag "${IMAGE_ID}" "${REGISTRY}/${IMAGE}:latest-${TAG}"
  docker push "${REGISTRY}/${IMAGE}:latest-${TAG}"
fi

# =======================================================
# Secondary velero-restic-restore-helper image.
# =======================================================

IMAGE=velero-restic-restore-helper
RESTORE_HELPER_IMAGE_ID="${REGISTRY}/${IMAGE}:${VERSION}-${TAG}"
echo "Building image with ID: $RESTORE_HELPER_IMAGE_ID"
cp -f $DIR/Dockerfile-velero-restic-restore-helper .
docker build \
  -t $RESTORE_HELPER_IMAGE_ID \
  -f Dockerfile-velero-restic-restore-helper \
  --build-arg target=$TARGET \
  --build-arg goarch=$GOARCH .

# Push push push
docker push $RESTORE_HELPER_IMAGE_ID
if [ $CIRCLE_BRANCH == 'master' ]; then
  docker tag "$RESTORE_HELPER_IMAGE_ID" "${REGISTRY}/${IMAGE}:latest-${TAG}"
  docker push "${REGISTRY}/${IMAGE}:latest-${TAG}"
fi
