package kite

import (
	"github.com/cloudfoundry/gosigar"
	"koding/newkite/protocol"
)

type Status struct{}

type Info struct {
	State       string `json:"state"`
	DiskUsage   uint64 `json:"diskUsage"`
	DiskTotal   uint64 `json:"diskTotal"`
	MemoryUsage uint64 `json:"memoryUsage"`
	MemoryTotal uint64 `json:"totalMemoryLimit"`
}

type memory struct {
	Usage uint64 `json:"memoryUsage"`
	Total uint64 `json:"memoryTotal"`
}

type disk struct {
	Usage uint64 `json:"diskUsage"`
	Total uint64 `json:"diskTotal"`
}

func memoryStats() *memory {
	m := new(memory)
	mem := sigar.Mem{}
	if err := mem.Get(); err == nil {
		m.Usage = mem.ActualUsed
		m.Total = mem.Total
	}

	return m
}

func diskStats() *disk {
	d := new(disk)
	space := sigar.FileSystemUsage{}
	if err := space.Get("/"); err == nil {
		d.Total = space.Total
		d.Usage = space.Used
	}
	return d
}

func (Status) Info(r *protocol.KiteDnodeRequest, result *Info) error {
	disk := diskStats()
	mem := memoryStats()

	info := &Info{
		State:       "RUNNING", // needed for client side compatibility
		DiskUsage:   disk.Usage,
		DiskTotal:   disk.Total,
		MemoryUsage: mem.Usage,
		MemoryTotal: mem.Total,
	}

	*result = *info
	return nil
}
