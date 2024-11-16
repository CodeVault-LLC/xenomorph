import winreg
from modules.antivirus.antivirus_shared import format_antivirus_info

class AntivirusWindows:
    def __init__(self):
        self.registry_paths = [
            r"SOFTWARE\Microsoft\Windows Defender",
            r"SOFTWARE\Avast Software\Avast",
            r"SOFTWARE\AVG\Antivirus",
            r"SOFTWARE\Avira\Antivirus",
            r"SOFTWARE\Bitdefender",
            r"SOFTWARE\ESET\ESET Security",
            r"SOFTWARE\KasperskyLab\AVP",
            r"SOFTWARE\McAfee",
            r"SOFTWARE\Symantec\Symantec Endpoint Protection",
            r"SOFTWARE\TrendMicro\PC-cillin",
            r"SOFTWARE\CheckPoint\ZoneAlarm",
            r"SOFTWARE\Panda Security",
        ]

    def run(self):
        antivirus_info = []
        for root in [winreg.HKEY_LOCAL_MACHINE, winreg.HKEY_CURRENT_USER]:
            for path in self.registry_paths:
                try:
                    with winreg.OpenKey(root, path) as key:
                        name = winreg.QueryValueEx(key, "DisplayName")[0] if "DisplayName" in path else path.split("\\")[-1]
                        version = winreg.QueryValueEx(key, "DisplayVersion")[0] if "DisplayVersion" in path else "Unknown"
                        publisher = winreg.QueryValueEx(key, "Publisher")[0] if "Publisher" in path else "Unknown"
                        antivirus_info.append(format_antivirus_info(name, version, publisher, path))
                except OSError:
                    # Skip if the registry key doesn't exist
                    continue
        return antivirus_info
