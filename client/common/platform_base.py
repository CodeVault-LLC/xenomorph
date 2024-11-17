import platform

class PlatformHandlerBase:
    # Setup for 3 platforms, Windows, MacOS, and Linux
    def __init__(self):
        self.platform = platform.system()

    def execute_windows(self):
        """Execute Windows specific code"""
        pass

    def execute_macos(self):
        """Execute MacOS specific code"""
        pass

    def execute_linux(self):
        """Execute Linux specific code"""
        pass

    def execute(self):
        if self.platform == "Windows":
            return self.execute_windows()
        elif self.platform == "Darwin":
            return self.execute_macos()
        elif self.platform == "Linux":
            return self.execute_linux()
        else:
            raise Exception("Unsupported platform")
