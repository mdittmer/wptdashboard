#!/bin/bash

#
# Prerequisites:
# Successful run of install.sh
#

set -ev

source $(readlink -f $(dirname "${0}"))/env.sh

PROTOS_IN_DIR="protos"
PYTHON_OUT_DIR="run/protos"
BQ_SCHEMA_IN_DIR="${BQ_SCHEMA_GEN_DIR}"
BQ_SCHEMA_OUT_DIR="bq-schema"

mkdir -p "${PYTHON_OUT_DIR}"
mkdir -p "${BQ_SCHEMA_OUT_DIR}"

if [[ ! -d "${PB_DIR}" ]]; then
  echo "ERROR: Did not find protobuf at ${PB_DIR}"
  exit 1
fi
if [[ ! -f "${PROTOC_BIN}" ]]; then
  echo "ERROR: Did not find protoc at ${PROTOC_BIN}"
  exit 1
fi
if [[ ! -f "${BQ_SCHEMA_GEN_BIN}" ]]; then
  echo "ERROR: Did not find BigQuery schema generation protoc plugin ${BQ_SCHEMA_GEN_BIN}"
  exit 1
fi

cd "${BASE_DIR}"
"${PROTOC_BIN}" -I"${PB_DIR}/src" -I"${BQ_SCHEMA_GEN_DIR}" -I"${PROTOS_IN_DIR}" --bq-schema_out="${BQ_SCHEMA_OUT_DIR}" "${PROTOS_IN_DIR}"/*.proto
"${PROTOC_BIN}" -I"${PB_DIR}/src" -I"${BQ_SCHEMA_GEN_DIR}" -I"${PROTOS_IN_DIR}" --python_out="${PYTHON_OUT_DIR}" "${BQ_SCHEMA_IN_DIR}"/*.proto "${PROTOS_IN_DIR}"/*.proto
