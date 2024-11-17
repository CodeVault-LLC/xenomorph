from common.platform_base import PlatformHandlerBase

class Antivirus(PlatformHandlerBase):
    def __init__(self):
        super().__init__()

    def execute_windows(self):
        from modules.antivirus.antivirus_windows import AntivirusWindows

        antivirus = AntivirusWindows()
        return antivirus.run()

    def execute_macos(self):
        from modules.antivirus.antivirus_mac import AntivirusMac

        antivirus = AntivirusMac()
        return antivirus.run()

    def execute_linux(self):
        from modules.antivirus.antivirus_linux import AntivirusLinux

        antivirus = AntivirusLinux()
        return antivirus.run()

    def execute(self):
        return super().execute()
