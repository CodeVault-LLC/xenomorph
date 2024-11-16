import os
from shutil import which
from modules.antivirus.antivirus_shared import format_antivirus_info

class AntivirusLinux:
    def __init__(self):
        self.common_programs = [
            "clamav",     # ClamAV
            "f-prot",     # F-Prot
            "sophos-av",  # Sophos
            "chkrootkit", # chkrootkit
            "rkhunter"    # rkhunter
        ]

    def run(self):
        antivirus_info = []
        for program in self.common_programs:
            path = which(program) or self.__check_config(program)
            if path:
                name = program.capitalize()
                antivirus_info.append(format_antivirus_info(name, source=path))
        return antivirus_info

    def __check_config(self, program):
        """Check if configuration files for the antivirus exist."""
        config_paths = [
            f"/etc/{program}/config",
            f"/etc/{program}.conf"
        ]
        for path in config_paths:
            if os.path.exists(path):
                return path
        return None
