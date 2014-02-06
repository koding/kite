package systeminfo

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
)

type procMem struct {
	Total      uint64
	Used       uint64
	Free       uint64
	ActualFree uint64
	ActualUsed uint64
}

func memoryStats() (*memory, error) {
	m := new(memory)
	mem := procMem{}

	err := mem.Get()
	if err != nil {
		return nil, err
	}

	m.Usage = mem.ActualUsed
	m.Total = mem.Total

	return m, nil
}

func (self *procMem) Get() error {
	var buffers, cached uint64
	table := map[string]*uint64{
		"MemTotal": &self.Total,
		"MemFree":  &self.Free,
		"Buffers":  &buffers,
		"Cached":   &cached,
	}

	if err := parseMeminfo(table); err != nil {
		return err
	}

	self.Used = self.Total - self.Free
	kern := buffers + cached
	self.ActualFree = self.Free + kern
	self.ActualUsed = self.Used - kern

	return nil
}

func parseMeminfo(table map[string]*uint64) error {
	return readFile("/proc/meminfo", func(line string) bool {
		fields := strings.Split(line, ":")

		if ptr := table[fields[0]]; ptr != nil {
			num := strings.TrimLeft(fields[1], " ")
			val, err := strtoull(strings.Fields(num)[0])
			if err == nil {
				*ptr = val * 1024
			}
		}

		return true
	})
}

func readFile(file string, handler func(string) bool) error {
	contents, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(bytes.NewBuffer(contents))

	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		if !handler(string(line)) {
			break
		}
	}

	return nil
}

func strtoull(val string) (uint64, error) {
	return strconv.ParseUint(val, 10, 64)
}
