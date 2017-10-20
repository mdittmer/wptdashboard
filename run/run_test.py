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

import unittest

from protos.test_summary_pb2 import TestSummary

from run import (
    report_to_summary,
    version_string_to_major_minor,
    proto_to_bq_tuple,
)

TEST_WPT_HASH = '0' * 40
TEST_WPT_COMMIT_TIME = 0

class TestRun(unittest.TestCase):

    def test_report_to_summary(self):
        actual = report_to_summary(
            TEST_WPT_HASH,
            TEST_WPT_COMMIT_TIME,
            {
                'results': [
                    {
                        'test': '/dom/a.html',
                        'status': 'OK',
                        'subtests': [
                            {'status': 'PASS'}
                        ]
                    },
                    {
                        'test': '/dom/b.html',
                        'status': 'OK',
                        'subtests': [
                            {'status': 'FAIL'}
                        ]
                    }
                ]
            }
        )
        self.assertEqual(actual, [
            TestSummary(long_wpt_hash=TEST_WPT_HASH,
                        wpt_commit_time=TEST_WPT_COMMIT_TIME,
                        name='/dom/a.html',
                        num_tests_passed=2,
                        num_tests_total=2),
            TestSummary(long_wpt_hash=TEST_WPT_HASH,
                        wpt_commit_time=TEST_WPT_COMMIT_TIME,
                        name='/dom/b.html',
                        num_tests_passed=1,
                        num_tests_total=2),
        ])

    def test_version_string_to_major_minor(self):
        with self.assertRaises(AssertionError):
            version_string_to_major_minor('')
        self.assertEqual(version_string_to_major_minor('1.1'), '1.1')
        self.assertEqual(version_string_to_major_minor('1.1.1'), '1.1')

    def test_proto_to_bq_tuple(self):
        NAME='/foo/bar.html'
        PASSED=2
        TOTAL=4
        summary = TestSummary(wpt_commit_time=TEST_WPT_COMMIT_TIME,
                              long_wpt_hash=TEST_WPT_HASH,
                              name=NAME,
                              num_tests_passed=PASSED,
                              num_tests_total=TOTAL)
        schema = [
            {'name': 'wpt_commit_time'},
            {'name': 'long_wpt_hash'},
            {'name': 'name'},
            {'name': 'num_tests_passed'},
            {'name': 'num_tests_total'},
        ]
        self.assertEquals(
            proto_to_bq_tuple(summary, schema),
            (
                TEST_WPT_COMMIT_TIME,
                TEST_WPT_HASH,
                NAME,
                PASSED,
                TOTAL,
            ),
        )

if __name__ == '__main__':
    unittest.main()
