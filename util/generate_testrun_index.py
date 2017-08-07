#!/usr/bin/python3

# Copyright 2017 Google Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import json
from google.cloud import storage

"""
Scans all WPT results directories then generates and uploads an index.

You must be logged into gcloud and a member of the wptdashboard project
in order for this script to work.
"""

GCP_PROJECT = 'wptdashboard'
RESULTS_BUCKET = 'wptd'


def main():
    storage_client = storage.Client(project=GCP_PROJECT)
    bucket = storage_client.get_bucket(RESULTS_BUCKET)

    # by_sha is an object where:
    # Key: a WPT commit SHA
    # Value: list of platform IDs the SHA was tested against
    by_sha = {}

    # by_platform is an object where:
    # Key: a platform ID
    # Value: list of WPT commit SHAs the platform was tested against
    by_platform = {}

    sha_directories = list_directory(bucket)

    for sha_directory in sha_directories:
        sha = sha_directory.replace('/', '')
        directories = list_directory(bucket, sha_directory)
        platform_directories = [
            prefix[len(sha_directory):].replace('/', '')
            for prefix in directories
        ]

        for platform in platform_directories:
            by_sha.setdefault(sha, [])
            by_sha[sha].append(platform)

            by_platform.setdefault(platform, [])
            by_platform[platform].append(sha)

    print('by_sha', by_sha)
    print('by_platform', by_platform)

    index = {
        'by_sha': by_sha,
        'by_platform': by_platform
    }

    filename = 'testruns-index.json'
    blob = bucket.blob(filename)
    blob.upload_from_string(json.dumps(index), content_type='application/json')

    print('Uploaded!')
    print('https://storage.googleapis.com/wptd/%s' % filename)


def list_directory(bucket, prefix=None):
    iterator = bucket.list_blobs(delimiter='/', prefix=prefix)
    response = iterator._get_next_page_response()
    return response['prefixes']


if __name__ == '__main__':
    main()
