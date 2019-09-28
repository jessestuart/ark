#!/bin/bash

# Copyright 2019 the Velero contributors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# gcr-push is invoked by the CI/CD system to deploy docker images to Google Container Registry.
# It will build images for all commits to master and all git tags.
# The highest, non-prerelease semantic version will also be given the `latest` tag.

set +x

if [[ -z "$TRAVIS" ]]; then
    echo "This script is intended to be run only on Travis." >&2
    exit 1
fi

# Return value is written into HIGHEST
HIGHEST=""
function highest_release() {
    # Loop through the tags since pre-release versions come before the actual versions.
    # Iterate til we find the first non-pre-release

    # This is not necessarily the most recently made tag; instead, we want it to be the highest semantic version.
    # The most recent tag could potentially be a lower semantic version, made as a point release for a previous series.
    # As an example, if v1.3.0 exists and we create v1.2.2, v1.3.0 should still be `latest`.
    # `git describe --tags $(git rev-list --tags --max-count=1)` would return the most recently made tag.

    for t in $(git tag -l --sort=-v:refname);
    do
        # If the tag has alpha, beta or rc in it, it's not "latest"
        if [[ "$t" == *"beta"* || "$t" == *"alpha"* || "$t" == *"rc"* ]]; then
            continue
        fi
        HIGHEST="$t"
        break
    done
}

if [ "$BRANCH" == "master" ]; then
    VERSION="$BRANCH"
elif [ ! -z "$TRAVIS_TAG" ]; then
    VERSION="$TRAVIS_TAG"
else
    # If we're not on master and we're not building a tag, exit early.
    exit 0
fi

# Calculate the latest release
highest_release

# Assume we're not tagging `latest` by default.
TAG_LATEST=false
if [[ "$TRAVIS_TAG" == "$HIGHEST" ]]; then
    TAG_LATEST=true
fi

openssl aes-256-cbc -K $encrypted_f58ab4413c21_key -iv $encrypted_f58ab4413c21_iv -in heptio-images-fac92d2303ac.json.enc -out heptio-images-fac92d2303ac.json -d
gcloud auth activate-service-account --key-file heptio-images-fac92d2303ac.json
gcloud auth configure-docker -q
unset GIT_HTTP_USER_AGENT

echo "Building and pushing container images."

VERSION="$VERSION" TAG_LATEST="$TAG_LATEST" make all-containers all-push
