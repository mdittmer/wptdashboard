#!/bin/bash

#
# File-watching development server script that run inside Docker development
# image.
#
# Presumed working directory in Docker instance is "/wptdashboard" mapped to
# source repository root directory.
#

set -e

DOCKER_INNER_DIR=$(dirname "$0")
source "${DOCKER_INNER_DIR}/../../logging.sh"
WPTDASHBOARD_DIR="${DOCKER_INNER_DIR}/../../.."

function stop() {
  warn "watch.sh: Recieved interrupt. Exiting..."
}

trap stop INT

PROTO_DIR=${PROTO_DIR:-"./protos"}
PY_DIR=${PY_DIR:-"./run"}
GO_DIR=${PY_DIR:-"./go"}

function compile_protos() {
  if make proto; then
    info "SUCCESS: Regen from protos"
  else
    error "FAILURE: Regen from protos failed"
  fi
}

function compile_go() {
  if make go_deps; then
    info "SUCCESS: Build go"
  else
    error "FAILURE: Build go failed"
  fi
}

function monitor() {
  EXT="${1}"
  DIR="${2}"
  FUNC="${3}"

  ${FUNC}
  inotifywait -r -m -e close_write,moved_to,create,delete,modify "${DIR}" | \
    while read -r SRC_DIR EVTS F; do
      if [[ ${F} =~ [.]${EXT}$ ]] && ! [[ ${F} =~ [#~] ]]; then
        pushd "${WPTDASHBOARD_DIR}" > /dev/null
        ${FUNC}
        popd > /dev/null
      else
        verbose "Unmonitoried file changed: ${SRC_DIR}${F}"
      fi
    done &
}

monitor "proto" "${PROTO_DIR}" "compile_protos"
monitor "go" "${GO_DIR}" "compile_go"

wait
