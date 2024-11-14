file_system_types = {
    "NTFS": "New Technology File System - Supports large files, file permissions, encryption, and compression.",
    "FAT32": "File Allocation Table 32 - Older file system, supports up to 4GB file size, widely compatible.",
    "exFAT": "Extended File Allocation Table - Similar to FAT32 but supports larger files, used in flash drives.",
    "ext4": "Fourth Extended File System - Commonly used in Linux, supports large files and journaling.",
    "HFS+": "Hierarchical File System Plus - Used in older macOS versions, supports large files and journaling.",
    "APFS": "Apple File System - Used in newer macOS versions, optimized for SSDs, supports encryption and snapshots."
}

def get_file_system_description(fstype):
    return file_system_types.get(fstype, "Unknown file system type")
