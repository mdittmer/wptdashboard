#!/bin/bash

APPLICATION_DEFAULT_CREDENTIALS_FILE=${APPLICATION_DEFAULT_CREDENTIALS_FILE:-"/home/jenkins/.config/gcloud/application_default_credentials.json"}
TOKEN="$(gcloud auth application-default print-access-token)"
STATUS="${?}"

set -e

DOCKER_INNER_DIR=$(dirname "$0")
source "${DOCKER_INNER_DIR}/../../logging.sh"

if [[ "${TOKEN}" == "" || "${STATUS}" != "0" ]]; then
  gcloud auth application-default login
  STATUS="${?}"
  if [ "${STATUS}" == "0" ]; then
    chmod a+r "${APPLICATION_DEFAULT_CREDENTIALS_FILE}"
    info "Application-default credentials now available"
  else
    error "Failed to obtain application-default credentails"
    exit "${STATUS}"
  fi
else
  info "Application-default credentials already available"
fi
