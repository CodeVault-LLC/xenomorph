def format_antivirus_info(name, version="Unknown", publisher="Unknown", source="Unknown"):
    """Standardize the antivirus information structure."""
    return {
        "Name": name,
        "Version": version,
        "Publisher": publisher,
        "Source": source
    }
