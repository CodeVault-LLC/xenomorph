from common.platform_base import PlatformHandlerBase
import os

class Browser(PlatformHandlerBase):
    def __init__(self):
        super().__init__()

    def execute_windows(self):
        from modules.browser.browser_windows import BrowserWindows

        browser = BrowserWindows()
        return browser.run()

    def execute_macos(self):
        from modules.browser.browser_mac import BrowserMac

        browser = BrowserMac()
        return browser.run()

    def execute_linux(self):
        from modules.browser.browser_linux import BrowserLinux

        browser = BrowserLinux()
        return browser.run()

    def execute(self) -> tuple:
        return super().execute()
