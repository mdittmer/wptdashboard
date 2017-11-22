import re

from protos.test_run_pb2 import TestRun
from protos import operating_system_pb2 as OS
from protos import browser_pb2 as Browser

class ProtoBuilder(object):
    def os_from_platform(self, platform):
        return getattr(OS, platform['os_name'].upper())

    def os_version_str_from_platform(self, platform):
        if platform['os_version'] == '*':
            return ''
        else:
            return platform['os_version']

    def browser_from_platform(self, platform):
        return getattr(Browser, platform['browser_name'].upper())

    def browser_version_str_from_platform(self, platform):
        return platform['browser_version']
