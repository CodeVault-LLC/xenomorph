from common.platform_base import PlatformHandlerBase

class Antivirus(PlatformHandlerBase):
    def __init__(self):
        super().__init__()

    def execute_windows(self):
        from modules.antivirus.antivirus_windows import AntivirusWindows

        antivirus = AntivirusWindows()
        antivirus.run()

    def execute_macos(self):
        from modules.antivirus.antivirus_mac import AntivirusMac

        antivirus = AntivirusMac()
        antivirus.run()
        pass

    def execute_linux(self):
        from modules.antivirus.antivirus_linux import AntivirusLinux

        antivirus = AntivirusLinux()
        antivirus.run()
        pass

    def execute(self):
        super().execute()
