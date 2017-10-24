#!/bin/bash

export SH_DIR=$(readlink -f $(dirname "${0}"))
export BASE_DIR="${SH_DIR}/.."
export THIRD_PARTY_DIR="${BASE_DIR}/third_party"
export PB_DIR="${THIRD_PARTY_DIR}/protobuf"
export PROTOC_DIR="${THIRD_PARTY_DIR}/protoc"
export PROTOC_BIN="${PROTOC_DIR}/bin/protoc"
export BQ_SCHEMA_GEN_DIR="${THIRD_PARTY_DIR}/protoc-gen-bq-schema"
export BQ_SCHEMA_GEN_BIN="${BQ_SCHEMA_GEN_DIR}/bin/protoc-gen-bq-schema"
