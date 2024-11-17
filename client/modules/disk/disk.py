from common.platform_base import PlatformHandlerBase
import psutil

class Disk(PlatformHandlerBase):
    def __init__(self):
        super().__init__()

    def __get_disk_info(self):
        data = []

        disk_info = psutil.disk_partitions()
        for disk in disk_info:
          disk_usage = psutil.disk_usage(disk.mountpoint)
          data += [f"Device: {disk.device} - Mountpoint: {disk.mountpoint} - FsType: {disk.fstype} - Total: {disk_usage.total} - Used: {disk_usage.used} - Free: {disk_usage.free} - Percent: {disk_usage.percent}"]

        return data

    def execute_windows(self):
      return self.__get_disk_info()

    def execute_macos(self):
      return self.__get_disk_info()

    def execute_linux(self):
      return self.__get_disk_info()

    def execute(self) -> list:
        return super().execute()
