import json
import os
import unittest

import util
from protos.test_run_pb2 import TestRun


class TestBrowsers(unittest.TestCase):
    def test_os_from_platform_browsers_json(self):
        wptd_path = '%s/..' % os.path.dirname(os.path.abspath(__file__))
        builder = util.ProtoBuilder()
        with open('%s/browsers.json' % wptd_path) as f:
            browsers = json.load(f)
        for platform_id in browsers:
            builder.os_from_platform(browsers[platform_id])

    def test_os_version_str_from_platform_browsers_json(self):
        wptd_path = '%s/..' % os.path.dirname(os.path.abspath(__file__))
        builder = util.ProtoBuilder()
        with open('%s/browsers.json' % wptd_path) as f:
            browsers = json.load(f)
        for platform_id in browsers:
            builder.os_version_str_from_platform(browsers[platform_id])

    def test_os_version_str_from_platform_standard(self):
        builder = util.ProtoBuilder()
        self.assertEqual(builder.os_version_str_from_platform({
            'os_version': '0.1',
        }), '0.1')

    def test_os_version_str_from_platform_wildcard(self):
        builder = util.ProtoBuilder()
        self.assertEqual(builder.os_version_str_from_platform({
            'os_version': '*',
        }), '')

    def test_browser_from_platform_browsers_json(self):
        wptd_path = '%s/..' % os.path.dirname(os.path.abspath(__file__))
        builder = util.ProtoBuilder()
        with open('%s/browsers.json' % wptd_path) as f:
            browsers = json.load(f)
        for platform_id in browsers:
            builder.browser_from_platform(browsers[platform_id])

    def test_browser_version_str_from_platform_browsers_json(self):
        wptd_path = '%s/..' % os.path.dirname(os.path.abspath(__file__))
        builder = util.ProtoBuilder()
        with open('%s/browsers.json' % wptd_path) as f:
            browsers = json.load(f)
        for platform_id in browsers:
            builder.browser_version_str_from_platform(browsers[platform_id])
