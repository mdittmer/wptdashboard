#!/bin/bash

#
# Prerequisites:
# 1. golang
# 2. `make`
# 3. `WPTDASHBOARD_OS` one of {"linux-x86_32", "linux-x86_64", "osx-x86_32",
#                              "osx-x86_64", "win32"}
#    or else default to "linux-x86_64"
# 4. `curl [url]` will download resource at [url]
# 5. `unzip [path]` will extract zip archive at [path]
#

set -e

source $(readlink -f $(dirname "${0}"))/env.sh

WPTDASHBOARD_OS=${WPTDASHBOARD_OS:-"linux-x86_64"}

PB_BRANCH="3.4.x"
PROTOC_ARCHIVE_NAME="protoc-3.4.0-${WPTDASHBOARD_OS}.zip"

mkdir -p "${THIRD_PARTY_DIR}"

function ensure_protobuf {
  if [[ ! -d "${PB_DIR}" ]]; then
    cd "${THIRD_PARTY_DIR}"
    git clone "git@github.com:google/protobuf.git"
  fi
  cd "${PB_DIR}"
  git checkout "${PB_BRANCH}"
  git pull
  cd "${BASE_DIR}"
}

function ensure_protoc {
  if [[ ! -d "${PROTOC_DIR}" ]]; then
    mkdir -p "${PROTOC_DIR}"
    cd "${PROTOC_DIR}"
    PROTOC_FILE_NAME=
    curl -L -o "${PROTOC_ARCHIVE_NAME}" "https://github.com/google/protobuf/releases/download/v3.4.0/${PROTOC_ARCHIVE_NAME}"
    unzip "${PROTOC_ARCHIVE_NAME}"
  fi
  if [[ ! -f "${PROTOC_BIN}" ]]; then
    echo "ERROR: Protoc installation failed"
    exit 1
  fi
  cd "${BASE_DIR}"
}

function ensure_protoc_bq {
  if [[ ! -d "${BQ_SCHEMA_GEN_DIR}" ]]; then
    cd "${THIRD_PARTY_DIR}"
    git clone git@github.com:GoogleCloudPlatform/protoc-gen-bq-schema.git
  fi
  cd "${BQ_SCHEMA_GEN_DIR}"
  make
  if [[ ! -f "${BQ_SCHEMA_GEN_BIN}" ]]; then
    echo "ERROR: Installation of BigQuery schema generation protoc plugin failed"
    exit 1
  fi
  cd "${BASE_DIR}"
}

ensure_protobuf
ensure_protoc
ensure_protoc_bq
