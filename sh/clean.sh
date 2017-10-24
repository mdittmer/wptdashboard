#!/bin/bash

set -e

source $(readlink -f $(dirname "${0}"))/env.sh

rm -f "${PYTHON_OUT_DIR}"/*_pb2.py*
