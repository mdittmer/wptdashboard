#!/bin/bash

DOCKER_DIR=$(dirname "$0")
source "${DOCKER_DIR}/../logging.sh"
WPTDASHBOARD_DIR=${WPTDASHBOARD_DIR:-"${DOCKER_DIR}/../.."}

info "Ensuring application-default credentials"
"${DOCKER_DIR}/su_i_exec.sh" /wptdashboard/util/docker/inner/dev_auth.sh
