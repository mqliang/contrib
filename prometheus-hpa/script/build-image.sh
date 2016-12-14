#!/usr/bin/env bash

#
# The script builds prometheus-hpa component container, see usage function for how to run
# the script. After build completes, following container will be built, i.e.
#   caicloud/prometheus-hpa:${IMAGE_TAG}
#
# By default, IMAGE_TAG is latest.

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(dirname "${BASH_SOURCE}")/..

function usage {
  echo -e "Usage:"
  echo -e "  ./build-image.sh [tag]"
  echo -e ""
  echo -e "Parameter:"
  echo -e " tag\tDocker image tag, treated as prometheus-hpa release version. If provided,"
  echo -e "    \tthe tag must be the form of vA.B.C, where A, B, C are digits, e.g."
  echo -e "    \tv1.0.1. If not provided, it will build images with tag 'latest'"
  echo -e ""
  echo -e "Environment variable:"
  echo -e " PUSH_TO_REGISTRY     \tPush images to caicloud registry or not, options: Y or N. Default value: ${PUSH_TO_REGISTRY}"
}

# -----------------------------------------------------------------------------
# Parameters for building docker image, see usage.
# -----------------------------------------------------------------------------
# Decide if we need to push the new images to caicloud registry.
PUSH_TO_REGISTRY=${PUSH_TO_REGISTRY:-"N"}

# Find image tag version, the tag is considered as release version.
if [[ "$#" == "1" ]]; then
  if [[ "$1" == "help" || "$1" == "--help" || "$1" == "-h" ]]; then
    usage
    exit 0
  else
    IMAGE_TAG=${1}
  fi
else
  IMAGE_TAG="latest"
fi

# -----------------------------------------------------------------------------
# Start Building containers
# -----------------------------------------------------------------------------
# Setup docker on Mac.
if [[ `uname` == "Darwin" ]]; then
  if [[ "$(which docker-machine)" != "" ]]; then
    eval "$(docker-machine env kube-dev)"
  elif [[ "$(which boot2docker)" != "" ]]; then
    eval "$(boot2docker shellinit)"
  fi
fi

echo "+++++ Start building prometheus-hpa"

cd ${ROOT}

# Build prometheus-hpa binary.
docker run --rm \
-v `pwd`:/go/src/k8s.io/contrib/prometheus-hpa \
index.caicloud.io/caicloud/golang:1.7 \
sh -c "cd /go/src/k8s.io/contrib/prometheus-hpa && go build -race ."
# Build prometheus-hpa container.
docker build -t index.caicloud.io/caicloud/prometheus-hpa-controller:${IMAGE_TAG} .

cd - > /dev/null

# Decide if we need to push images to caicloud registry.
if [[ "$PUSH_TO_REGISTRY" == "Y" ]]; then
  echo ""
  echo "+++++ Start pushing prometheus-hpa"
  docker push index.caicloud.io/caicloud/prometheus-hpa-controller:${IMAGE_TAG}
fi

echo "Successfully built docker image index.caicloud.io/caicloud/prometheus-hpa-controller:${IMAGE_TAG}"

# A reminder for creating Github release.
if [[ "$#" == "1" && $1 =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo -e "Finish building release ; if this is a formal release, please remember"
  echo -e "to create a release tag at Github at: https://github.com/caicloud/contrib/prometheus-hpa/releases"
fi
