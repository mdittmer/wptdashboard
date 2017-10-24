#!/bin/bash

set -e

source $(readlink -f $(dirname "${0}"))/env.sh

if [ "${WPTDASHBOARD_USE_SUDO}" != "" ]; then
  sudo rm -rf "${THIRD_PARTY_DIR}"
else
  rm -rf "${THIRD_PARTY_DIR}"
fi
