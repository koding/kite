package kite

import (
	"github.com/cloudfoundry/gosigar"
	"koding/newkite/protocol"
	"os/exec"
	"runtime"
	"strings"
)

type Status struct{}

type Info struct {
	State       string `json:"state"`
	DiskUsage   string `json:"diskUsage"`
	DiskTotal   string `json:"diskTotal"`
	MemoryUsage uint64 `json:"memoryUsage"`
	MemoryTotal uint64 `json:"memoryTotal"`
}

type memory struct {
	Usage uint64 `json:"memoryUsage"`
	Total uint64 `json:"memoryTotal"`
}

type disk struct {
	Usage string `json:"diskUsage"`
	Total string `json:"diskTotal"`
}

func memoryStats() *memory {
	m := new(memory)
	mem := sigar.Mem{}
	if err := mem.Get(); err == nil {
		m.Usage = mem.ActualUsed / 1024 / 1024
		m.Total = mem.Total / 1024 / 1024
	}

	return m
}

func diskStats() *disk {
	if runtime.GOOS != "darwin" {
		return nil // only darwin
	}

	out, _ := exec.Command("bash", "-c", "df -H | grep '\\/dev\\/'|  awk '{print $2, $3}'").CombinedOutput()
	diskStats := strings.Split(strings.TrimSpace(string(out)), " ")

	d := new(disk)
	d.Total = diskStats[0]
	d.Usage = diskStats[1]
	return d
}

func (Status) Info(r *protocol.KiteDnodeRequest, result *Info) error {
	disk := diskStats()
	mem := memoryStats()

	info := &Info{
		State:       "Running", // needed for client side compatibility
		DiskUsage:   disk.Usage,
		DiskTotal:   disk.Total,
		MemoryUsage: mem.Usage,
		MemoryTotal: mem.Total,
	}

	*result = *info
	return nil
}
