#!/bin/bash

RUN_DIR=$(readlink -f $(dirname "$0"))

# TODO(markdittmer): Setup dep on protobuf, protoc, protoc-gen-bq-schema
# installation locations.

# WIP: From my local setup in root repo dir...

# Generate Python
protoc -I../protobuf/src -I../protoc-gen-bq-schema -Iprotos --python_out=run/protos ../protoc-gen-bq-schema/bq_table_name.proto protos/test_summary.proto

# Generate BigQuery schema
protoc -I../protobuf/src -I../protoc-gen-bq-schema -Iprotos --bq-schema_out=bq-schema protos/test_summary.proto
