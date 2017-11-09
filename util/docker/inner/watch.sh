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

function stop() {
  warn "watch.sh: Recieved interrupt. Exiting..."
}

trap stop INT

PB_LIB=${PB_LIB:-"../protobuf/src"}
PROTOS=${PROTOS:-"./protos"}
BQ_LIB=${BQ_LIB:-"../protoc-gen-bq-schema"}
BQ_OUT=${BQ_OUT:-"./bq-schema"}
PY_OUT=${PY_OUT:-"./run/protos"}
GO_OUT=${GO_OUT:-"./go/wptdashboard/protos"}
# TODO: Use GoogleCloudPlatform/mdittmer/protoc-gen-bq-schema after
# https://github.com/GoogleCloudPlatform/protoc-gen-bq-schema/pull/4
# lands
GO_PKGMAP=${GO_PKGMAP:-"Mbq_table_name.proto=github.com/mdittmer/protoc-gen-bq-schema/protos"}

mkdir -p "${BQ_OUT}"
mkdir -p "${PY_OUT}"
mkdir -p "${GO_OUT}"

sleep 1

function compile_protos() {
  info "START: Regen from protos"
  if ! protoc -I"${PB_LIB}" -I"${BQ_LIB}" -I"${PROTOS}" \
      --bq-schema_out="${BQ_OUT}" \
      "${PROTOS}"/*.proto; then
    error "FAILURE: Regen BigQuery schema from protos failed"
  fi
  if ! protoc -I"${PB_LIB}" -I"${BQ_LIB}" -I"${PROTOS}" \
      --python_out="${PY_OUT}" \
      "${BQ_LIB}"/*.proto "${PROTOS}"/*.proto; then
    error "FAILURE: Regen Python code from protos failed"
  fi
  if ! protoc -I"${PB_LIB}" -I"${BQ_LIB}" -I"${PROTOS}" \
      --go_out="${GO_PKGMAP}:${GO_OUT}" \
      "${PROTOS}"/*.proto; then
    error "FAILURE: Regen Go code from protos failed"
  fi
  info "FINISH: Regen from protos"
}

compile_protos

inotifywait -r -m -e close_write,moved_to,create,delete,modify "${PROTOS}" | \
    while read -r DIR EVTS F; do
      if [[ ${F} =~ [.]proto$ ]] && ! [[ ${F} =~ [#~] ]]; then
        compile_protos
      else
        verbose "Non-proto file changed: ${DIR}${F}"
      fi
    done
