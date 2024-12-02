import os
import sys
import subprocess
import mimetypes
from typing import Any

def isUserAdmin() -> bool:
    """Checks if the current user is an administrator"""
    if os.name == 'nt':
        # Windows
        import ctypes
        try:
            return ctypes.windll.shell32.IsUserAnAdmin() != 0
        except:  # noqa: E722
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
        result: int = ctypes.windll.shell32.ShellExecuteW(
            None, "runas", sys.executable, " ".join(sys.argv), None, 1
        )
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

def get_external_ip() -> str:
    """Returns the external IP address of the client"""
    try:
        return subprocess.check_output(["curl", "-s", "ifconfig.me"]).decode().strip()
    except Exception as e:
        return str(e)

def get_gpu_info() -> str:
    """Returns the GPU information"""
    if os.name == 'nt':
        # Windows
        import wmi
        w: Any = wmi.WMI()
        try:
            return w.Win32_VideoController()[0].name
        except Exception as e:
            return str(e)
    else:
        try:
            from gputil import getGPUs
            gpu_list: Any = getGPUs()
            return gpu_list[0].name if gpu_list else "No GPUs found"
        except ImportError:
            return "N/A"

def get_mime_type(file_path: str) -> str:
    """Returns the MIME type of a file."""
    return mimetypes.guess_type(file_path)[0] or "application/octet-stream"
