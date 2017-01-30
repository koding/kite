package systeminfo

import (
	"syscall"
	"unsafe"
)

func diskStats() (*disk, error) {
	d := new(disk)
	stat := new(syscall.Statfs_t)

	if err := syscall.Statfs("/", stat); err != nil {
		return nil, err
	}

	bsize := uint64(stat.F_bsize) / 512

	d.Total = (uint64(stat.F_blocks) * bsize) >> 1
	free := (uint64(stat.F_bfree) * bsize) >> 1
	d.Usage = d.Total - free

	return d, nil
}

func memoryStats() (*memory, error) {
	m := new(memory)

	// get page size
	pgsz_mib := []uint32{
		6, // CTL_HW
		7, // HW_PAGESIZE
	}

	pgsz := uintptr(0)

	sz := uintptr(unsafe.Sizeof(pgsz))

	_, _, err := syscall.Syscall6(syscall.SYS___SYSCTL, uintptr(unsafe.Pointer(&pgsz_mib[0])), 2, uintptr(unsafe.Pointer(&pgsz)), uintptr(unsafe.Pointer(&sz)), 0, 0)
	if err != 0 {
		return nil, err
	}

	// get memory stats
	vm_mib := []uint32{
		2, // CTL_VM
		1, // VM_METER
	}

	// from OpenBSD /usr/include/sys/vmmeter.h
	var vmtotal struct {
		t_rq     uint16 /* length of the run queue */
		t_dw     uint16 /* jobs in ``disk wait'' (neg priority) */
		t_pw     uint16 /* jobs in page wait */
		t_sl     uint16 /* jobs sleeping in core */
		t_sw     uint16 /* swapped out runnable/short block jobs */
		t_vm     uint32 /* total virtual memory */
		t_avm    uint32 /* active virtual memory */
		t_rm     uint32 /* total real memory in use */
		t_arm    uint32 /* active real memory */
		t_vmshr  uint32 /* shared virtual memory */
		t_avmshr uint32 /* active shared virtual memory */
		t_rmshr  uint32 /* shared real memory */
		t_armshr uint32 /* active shared real memory */
		t_free   uint32 /* free memory pages */
	}

	sz = uintptr(unsafe.Sizeof(vmtotal))

	_, _, err = syscall.Syscall6(syscall.SYS___SYSCTL, uintptr(unsafe.Pointer(&vm_mib[0])), 2, uintptr(unsafe.Pointer(&vmtotal)), uintptr(unsafe.Pointer(&sz)), 0, 0)
	if err != 0 {
		return nil, err
	}

	m.Total = uint64(vmtotal.t_avm+vmtotal.t_free) * uint64(pgsz)
	m.Usage = uint64(vmtotal.t_avm) * uint64(pgsz)

	return m, nil
}
