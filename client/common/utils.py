import os
import sys
import subprocess

def isUserAdmin() -> bool:
    """Checks if the current user is an administrator"""
    if os.name == 'nt':
        # Windows
        import ctypes
        try:
            return ctypes.windll.shell32.IsUserAnAdmin()
        except:
            return False
    else:
        # macOS and Linux
        if os.geteuid() == 0 or os.getuid() == 0:
            return True
        return False

def runAsAdmin() -> None:
    """Relaunches the current script with admin privileges"""
    if os.name == 'nt':
        # Windows
        import ctypes
        result = ctypes.windll.shell32.ShellExecuteW(None, "runas", sys.executable, " ".join(sys.argv), None, 1)
        if result > 32:
            sys.exit(0)
        else:
            raise PermissionError("User denied request for admin privileges")
    else:
        # macOS and Linux
        if isUserAdmin():
            return
        try:
            subprocess.check_call(['sudo', sys.executable] + sys.argv)
            sys.exit(0)
        except subprocess.CalledProcessError:
            raise PermissionError("User denied request for admin privileges")

def get_gpu_info() -> str:
    """Returns the GPU information"""
    if os.name == 'nt':
        # Windows
        import wmi
        w = wmi.WMI()
        try:
            return w.Win32_VideoController()[0].name
        except Exception as e:
            return str(e)
    else:
        try:
            from gputil import getGPUs
            return getGPUs()[0].name
        except ImportError:
            return "N/A"

file_system_types = {
    "NTFS": "New Technology File System - Supports large files, file permissions, encryption, and compression.",
    "FAT32": "File Allocation Table 32 - Older file system, supports up to 4GB file size, widely compatible.",
    "exFAT": "Extended File Allocation Table - Similar to FAT32 but supports larger files, used in flash drives.",
    "ext4": "Fourth Extended File System - Commonly used in Linux, supports large files and journaling.",
    "HFS+": "Hierarchical File System Plus - Used in older macOS versions, supports large files and journaling.",
    "APFS": "Apple File System - Used in newer macOS versions, optimized for SSDs, supports encryption and snapshots."
}

def get_file_system_description(fstype):
    return file_system_types.get(fstype, "Unknown file system type")
