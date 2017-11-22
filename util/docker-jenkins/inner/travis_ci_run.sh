#!/bin/bash

# This file runs inside a WPT testrun container
# in the context of Travis CI.

set -ex

export BUILD_PATH="${WPTD_PATH}"
# Run a small directory (4 tests)
export RUN_PATH=battery-status
export WPT_SHA=$(cd $WPT_PATH && git rev-parse HEAD)
export WPT_TIMETAMP=$(cd $WPT_PATH && git log -1 --date=unix --format=%cd)

export PLATFORM_ID=firefox-57.0-linux

mkdir -p "${WPTD_OUT_PATH}"
python "${WPTD_PATH}/run/jenkins.py"
