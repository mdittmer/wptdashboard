#!/bin/bash

DOCKER_DIR=$(dirname "$0")
source "${DOCKER_DIR}/../logging.sh"
WPTDASHBOARD_DIR=${WPTDASHBOARD_DIR:-"${DOCKER_DIR}/../.."}

info "Ensuring application-default credentials"
docker exec -it -u 0:0 wptd-dev-instance /wptdashboard/util/docker-dev/inner/dev_auth.sh
