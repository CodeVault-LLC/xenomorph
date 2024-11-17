import os
import sys
import psutil

Browsers = {}

def generate():
  if sys.platform == "win32":
    local_appdata = os.getenv('LOCALAPPDATA')
    default_appdata = os.getenv('APPDATA')

    Browsers["chrome"] = local_appdata + \
        "\\Google\\Chrome\\User Data\\"
    Browsers["firefox"] = local_appdata + \
        "\\Mozilla\\Firefox\\Profiles\\"
    Browsers["opera"] = default_appdata + \
        "\\Opera Software\\Opera Stable\\"
    Browsers["edge"] = local_appdata + \
        "\\Microsoft\\Edge\\User Data\\"
    Browsers["brave"] = local_appdata + \
        "\\BraveSoftware\\Brave-Browser\\User Data\\"
    Browsers["vivaldi"] = local_appdata + \
        "\\Vivaldi\\User Data\\"
    Browsers["safari"] = local_appdata + "\\Apple Computer\\Safari\\"
    Browsers["tor"] = local_appdata + \
        "\\Tor Browser\\Browser\\TorBrowser\\Data\\Browser\\profile."
    Browsers["maxthon"] = local_appdata + "\\Maxthon3\\Users\\"
    Browsers["epic"] = local_appdata + \
        "\\Epic Privacy Browser\\User Data\\"
    Browsers["avast"] = local_appdata + \
        "\\AVAST Software\\Browser\\User Data\\"
    Browsers["chromium"] = local_appdata + \
        "\\Chromium\\User Data\\"
    Browsers["comodo"] = local_appdata + \
        "\\Comodo\\Dragon\\User Data\\"
    Browsers["torch"] = local_appdata + "\\Torch\\User Data\\"
    Browsers["360"] = local_appdata + \
        "\\360Browser\\Browser\\User Data\\"
    Browsers["blisk"] = local_appdata + "\\Blisk\\User Data\\"
    Browsers["brave"] = local_appdata + \
        "\\BraveSoftware\\Brave-Browser\\User Data\\"
    Browsers["centbrowser"] = local_appdata + \
        "\\CentBrowser\\User Data\\"
    Browsers["chromium"] = local_appdata + \
        "\\Chromium\\User Data\\"
    Browsers["comodo"] = local_appdata + \
        "\\Comodo\\Dragon\\User Data\\"
    Browsers["cyberfox"] = local_appdata + \
        "\\8pecxstudios\\Cyberfox\\Profiles\\"
  elif sys.platform == "darwin":
    Browsers["chrome"] = os.path.expanduser(
        "~/Library/Application Support/Google/Chrome/")
    Browsers["firefox"] = os.path.expanduser(
        "~/Library/Application Support/Firefox/Profiles/")
    Browsers["opera"] = os.path.expanduser(
        "~/Library/Application Support/com.operasoftware.Opera/")
    Browsers["edge"] = os.path.expanduser(
        "~/Library/Application Support/Microsoft Edge/")
    Browsers["brave"] = os.path.expanduser(
        "~/Library/Application Support/BraveSoftware/Brave-Browser/")
    Browsers["vivaldi"] = os.path.expanduser(
        "~/Library/Application Support/Vivaldi/")
    Browsers["safari"] = os.path.expanduser(
        "~/Library/Safari/")
    Browsers["tor"] = os.path.expanduser(
        "~/Library/Application Support/TorBrowser-Data/Browser/profile.")
    Browsers["maxthon"] = os.path.expanduser(
        "~/Library/Application Support/Maxthon3/Users/")
    Browsers["epic"] = os.path.expanduser(
        "~/Library/Application Support/Epic Privacy Browser/")
    Browsers["avast"] = os.path.expanduser(
        "~/Library/Application Support/AVAST Software/Browser/")
    Browsers["chromium"] = os.path.expanduser(
        "~/Library/Application Support/Chromium/")
    Browsers["comodo"] = os.path.expanduser(
        "~/Library/Application Support/Comodo/Dragon/")
    Browsers["torch"] = os.path.expanduser(
        "~/Library/Application Support/Torch/")
    Browsers["360"] = os.path.expanduser(
        "~/Library/Application Support/360Browser/Browser/")
    Browsers["blisk"] = os.path.expanduser(
        "~/Library/Application Support/Blisk/")
    Browsers["brave"] = os.path.expanduser(
        "~/Library/Application Support/BraveSoftware/Brave-Browser/")
    Browsers["centbrowser"] = os.path.expanduser(
        "~/Library/Application Support/CentBrowser/")
    Browsers["chromium"] = os.path.expanduser(
        "~/Library/Application Support/Chromium/")
    Browsers["comodo"] = os.path.expanduser(
        "~/Library/Application Support/Comodo/Dragon/")
    Browsers["cyberfox"] = os.path.expanduser(
        "~/Library/Application Support/8pecxstudios/Cyberfox/Profiles/")
  elif sys.platform == "linux":
    Browsers["chrome"] = os.path.expanduser("~/.config/google-chrome/")
    Browsers["firefox"] = os.path.expanduser("~/.mozilla/firefox/")
    Browsers["opera"] = os.path.expanduser("~/.config/opera/")
    Browsers["edge"] = os.path.expanduser("~/.config/microsoft-edge/")
    Browsers["brave"] = os.path.expanduser("~/.config/BraveSoftware/Brave-Browser/")
    Browsers["vivaldi"] = os.path.expanduser("~/.config/vivaldi/")
    Browsers["safari"] = os.path.expanduser("~/.config/safari/")
    Browsers["tor"] = os.path.expanduser("~/.config/tor-browser/")
    Browsers["maxthon"] = os.path.expanduser("~/.config/maxthon3/")
    Browsers["epic"] = os.path.expanduser("~/.config/epic-privacy-browser/")
    Browsers["avast"] = os.path.expanduser("~/.config/avast-software/browser/")
    Browsers["chromium"] = os.path.expanduser("~/.config/chromium/")
    Browsers["comodo"] = os.path.expanduser("~/.config/comodo/dragon/")
    Browsers["torch"] = os.path.expanduser("~/.config/torch/")
    Browsers["360"] = os.path.expanduser("~/.config/360browser/browser/")
    Browsers["blisk"] = os.path.expanduser("~/.config/blisk/")
    Browsers["brave"] = os.path.expanduser("~/.config/bravesoftware/brave-browser/")
    Browsers["centbrowser"] = os.path.expanduser("~/.config/centbrowser/")
    Browsers["chromium"] = os.path.expanduser("~/.config/chromium/")
    Browsers["comodo"] = os.path.expanduser("~/.config/comodo/dragon/")
    Browsers["cyberfox"] = os.path.expanduser("~/.config/8pecxstudios/cyberfox/profiles/")

def get_installed_browsers():
  browsers = []
  for browser in Browsers:
    if os.path.exists(Browsers[browser]):
      browsers.append(browser)
  return browsers

generate()

def close_browser_process(self, browser_name: str):
  """Close the browser process to safely access the database."""
  for proc in psutil.process_iter():
      if proc.name().lower() == browser_name.lower():
          proc.kill()
