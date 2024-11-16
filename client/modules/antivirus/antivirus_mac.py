import os
from modules.antivirus.antivirus_shared import format_antivirus_info

class AntivirusMac:
    def __init__(self):
        self.common_paths = [
            "/Applications/Avast Security.app",
            "/Applications/Bitdefender.app",
            "/Applications/Norton 360.app",
            "/Applications/Malwarebytes.app",
            "/Applications/Sophos.app"
        ]

    def run(self):
        antivirus_info = []
        for path in self.common_paths:
            if os.path.exists(path):
                name = os.path.basename(path).replace(".app", "")
                antivirus_info.append(format_antivirus_info(name, source=path))
        return antivirus_info
