#!/bin/bash

set -e

source $(readlink -f $(dirname "${0}"))/env.sh

rm -rf "${THIRD_PARTY_DIR}"
