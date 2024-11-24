import platform
import os
import base64
import subprocess
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from cryptography.hazmat.primitives.asymmetric import padding
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.serialization import load_pem_public_key


class Sec():
    def __init__(self):
        pass

    def aes_encrypt(self, data, key):
        iv = os.urandom(16)  # Generate a random IV
        cipher = Cipher(algorithms.AES(key), modes.CFB(iv))
        encryptor = cipher.encryptor()
        ciphertext = encryptor.update(data) + encryptor.finalize()
        return iv + ciphertext  # Include IV with the ciphertext

    def save_public_key(self, public_key_base64):
      """
      Save the server's public key securely based on the operating system.
      """
      public_key_pem = base64.b64decode(public_key_base64)
      system_info = platform.system()

      if system_info == "Windows":
          self._save_key_windows(public_key_pem)
      elif system_info == "Linux":
          self._save_key_linux(public_key_pem)
      elif system_info == "Darwin":  # macOS
          self._save_key_mac(public_key_pem)
      else:
          raise Exception(f"Unsupported operating system: {system_info}")

    def _save_key_windows(self, public_key_pem):
        """
        Save the public key to the Windows Registry.
        """
        try:
            import winreg
            key = winreg.CreateKey(winreg.HKEY_CURRENT_USER, r"Software\SecureClient")
            winreg.SetValueEx(key, "PublicKey", 0, winreg.REG_BINARY, public_key_pem)
            winreg.CloseKey(key)
            print("Public key saved to Windows Registry.")
        except Exception as e:
            print(f"Failed to save public key to registry: {e}")
            raise

    def _save_key_linux(self, public_key_pem):
        """
        Save the public key to a protected file on Linux.
        """
        try:
            secure_dir = os.path.expanduser("~/.secure_config")
            os.makedirs(secure_dir, exist_ok=True)
            file_path = os.path.join(secure_dir, "server_public_key.pem")
            with open(file_path, "wb") as f:
                f.write(public_key_pem)
            os.chmod(file_path, 0o600)  # Only the owner can read/write
            print(f"Public key saved to {file_path}.")
        except Exception as e:
            print(f"Failed to save public key to file: {e}")
            raise

    def _save_key_mac(self, public_key_pem):
        """
        Save the public key to the macOS Keychain (or fallback to file).
        """
        try:
            keychain_label = "SecureClientPublicKey"
            # Use macOS security command to store in Keychain
            process = subprocess.run(
                ["security", "add-generic-password", "-s", keychain_label, "-a", "SecureClient", "-w", public_key_pem.decode()],
                check=True,
                stderr=subprocess.PIPE
            )
            print("Public key saved to macOS Keychain.")
        except subprocess.CalledProcessError:
            # Fallback to Linux-like file storage
            print("Keychain storage failed; using file-based storage.")
            self._save_key_linux(public_key_pem)

    def load_public_key(self) -> None:
        """
        Load the server's public key securely based on the operating system.
        """
        system_info = platform.system()
        public_key_pem_bytes = None

        try:
            if system_info == "Windows":
                public_key_pem_bytes = self._load_key_windows()
            elif system_info == "Linux":
                public_key_pem_bytes = self._load_key_linux()
            elif system_info == "Darwin":  # macOS
                public_key_pem_bytes = self._load_key_mac()
            else:
                raise Exception(f"Unsupported operating system: {system_info}")

            # Load the key using PyCryptodome's RSA module
            self.public_key = load_pem_public_key(public_key_pem_bytes)

        except Exception as e:
            print(f"Failed to load public key: {e}")
            raise

    def _load_key_windows(self):
        import winreg
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, r"Software\SecureClient")
        public_key_pem, _ = winreg.QueryValueEx(key, "PublicKey")
        winreg.CloseKey(key)
        # Ensure the public key is in bytes and correctly formatted
        if isinstance(public_key_pem, str):
            public_key_pem = public_key_pem.encode('utf-8')
        return public_key_pem

    def _load_key_linux(self):
        secure_dir = os.path.expanduser("~/.secure_config")
        file_path = os.path.join(secure_dir, "server_public_key.pem")
        with open(file_path, "rb") as f:
            return f.read()

    def _load_key_mac(self):
        keychain_label = "SecureClientPublicKey"
        result = subprocess.run(
            ["security", "find-generic-password", "-s", keychain_label, "-a", "SecureClient", "-w"],
            check=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE
        )
        return result.stdout.strip()

    def encrypt(self, data: str) -> bytes:
        """
        Encrypts the given data for secure transmission.
        Returns the combined encrypted data.
        """
        if not hasattr(self, "public_key"):
            return None

        encoded_data = data.encode("utf-8")

        aes_key = os.urandom(32)

        encrypted_message = self.aes_encrypt(encoded_data, aes_key)

        encrypted_aes_key = self.public_key.encrypt(
            aes_key,
            padding.OAEP(
                mgf=padding.MGF1(algorithm=hashes.SHA256()),
                algorithm=hashes.SHA256(),
                label=None,
            ),
        )

        return encrypted_aes_key + b"||" + encrypted_message
