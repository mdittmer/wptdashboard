#!/bin/bash

set -e

source $(readlink -f $(dirname "${0}"))/env.sh

PROTOS_IN_DIR="${BASE_DIR}/protos"
BQ_SCHEMA_IN_DIR="${BQ_SCHEMA_GEN_DIR}"

mkdir -p "${PYTHON_OUT_DIR}"
mkdir -p "${BQ_SCHEMA_OUT_DIR}"

if [[ ! -d "${PB_DIR}" ]]; then
  echo "Did not find protobuf at ${PB_DIR}"
  echo "Running installdeps.sh"
  "${SH_DIR}/installdeps.sh"
  if [[ ! -d "${PB_DIR}" ]]; then
    echo "ERROR: Did not find protobuf at ${PB_DIR}"
    exit 1
  fi
fi
if [[ ! -f "${PROTOC_BIN}" ]]; then
  echo "Did not find protoc at ${PROTOC_BIN}"
  echo "Running installdeps.sh"
  "${SH_DIR}/installdeps.sh"
  if [[ ! -f "${PROTOC_BIN}" ]]; then
    echo "ERROR: Did not find protoc at ${PROTOC_BIN}"
    exit 1
  fi
fi
if [[ ! -f "${BQ_SCHEMA_GEN_BIN}" ]]; then
  echo "Did not find BigQuery schema generation protoc plugin ${BQ_SCHEMA_GEN_BIN}"
  echo "Running installdeps.sh"
  "${SH_DIR}/installdeps.sh"
  if [[ ! -f "${BQ_SCHEMA_GEN_BIN}" ]]; then
    echo "ERROR: Did not find BigQuery schema generation protoc plugin ${BQ_SCHEMA_GEN_BIN}"
    exit 1
  fi
fi

cd "${BASE_DIR}"
"${PROTOC_BIN}" -I"${PB_DIR}/src" -I"${BQ_SCHEMA_GEN_DIR}" -I"${PROTOS_IN_DIR}" --bq-schema_out="${BQ_SCHEMA_OUT_DIR}" "${PROTOS_IN_DIR}"/*.proto
"${PROTOC_BIN}" -I"${PB_DIR}/src" -I"${BQ_SCHEMA_GEN_DIR}" -I"${PROTOS_IN_DIR}" --python_out="${PYTHON_OUT_DIR}" "${BQ_SCHEMA_IN_DIR}"/*.proto "${PROTOS_IN_DIR}"/*.proto

if [ "$(find ${PYTHON_OUT_DIR} -type f | grep '_pb2[.]py$')" == "" ]; then
  echo "ERROR: No Python output from protobufs"
  exit 1
fi
if [ "$(find ${BQ_SCHEMA_OUT_DIR} -type f | grep '[.]schema$')" == "" ]; then
  echo "ERROR: No BigQuery schema output from protobufs"
  exit 1
fi
