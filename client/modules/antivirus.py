import winreg

def get_antivirus_info():
    antivirus_registry_paths = [
        r"SOFTWARE\Microsoft\Windows Defender",  # Windows Defender
        r"SOFTWARE\Avast Software\Avast",        # Avast
        r"SOFTWARE\AVG\Antivirus",               # AVG
        r"SOFTWARE\Avira\Antivirus",             # Avira
        r"SOFTWARE\Bitdefender",                 # Bitdefender
        r"SOFTWARE\ESET\ESET Security",          # ESET
        r"SOFTWARE\KasperskyLab\AVP",            # Kaspersky
        r"SOFTWARE\McAfee",                      # McAfee
        r"SOFTWARE\Symantec\Symantec Endpoint Protection", # Symantec/Norton
        r"SOFTWARE\TrendMicro\PC-cillin",        # Trend Micro
        r"SOFTWARE\CheckPoint\ZoneAlarm",        # ZoneAlarm
        r"SOFTWARE\Panda Security",              # Panda Security
    ]

    antivirus_info = []
    for root in [winreg.HKEY_LOCAL_MACHINE, winreg.HKEY_CURRENT_USER]:
        for path in antivirus_registry_paths:
            try:
                with winreg.OpenKey(root, path) as key:
                    # Retrieve basic information about the antivirus if available
                    name, version, publisher = None, None, None
                    try:
                        name = winreg.QueryValueEx(key, "DisplayName")[0]
                    except OSError:
                        name = path.split("\\")[-1]  # Use the path name as a fallback

                    try:
                        version = winreg.QueryValueEx(key, "DisplayVersion")[0]
                    except OSError:
                        version = "Unknown"

                    try:
                        publisher = winreg.QueryValueEx(key, "Publisher")[0]
                    except OSError:
                        publisher = "Unknown"

                    antivirus_info.append({
                        "Name": name,
                        "Version": version,
                        "Publisher": publisher,
                        "RegistryPath": path
                    })
            except OSError:
                # Continue to the next path if the registry key does not exist
                pass

    return antivirus_info
