// +build darwin freebsd linux netbsd

package systeminfo

import "syscall"

func diskStats() (*disk, error) {
	d := new(disk)
	stat := new(syscall.Statfs_t)

	if err := syscall.Statfs("/", stat); err != nil {
		return nil, err
	}

	bsize := stat.Bsize / 512

	d.Total = (uint64(stat.Blocks) * uint64(bsize)) >> 1
	free := (uint64(stat.Bfree) * uint64(bsize)) >> 1
	d.Usage = d.Total - free

	return d, nil
}
