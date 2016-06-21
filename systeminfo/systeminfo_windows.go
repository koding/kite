package systeminfo

import (
	"syscall"
	"unsafe"
)

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetDiskFreeSpaceExW  = modkernel32.NewProc("GetDiskFreeSpaceExW")
	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
)

// diskStats returns information about the amount of space that is available on
// a current disk volume for the user who calls this function.
func diskStats() (*disk, error) {
	var (
		freeBytes  uint64 = 0
		totalBytes uint64 = 0
	)

	// windows sets return value to 0 when function fails.
	ret, _, err := procGetDiskFreeSpaceExW.Call(
		0,
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		0,
	)
	if ret == 0 {
		return nil, err
	}

	// diskStats functions from other platforms return disk usage in kiB.
	return &disk{
		Usage: (totalBytes - freeBytes) / 1024,
		Total: totalBytes / 1024,
	}, nil
}

// memoryStatus retrieves information about the system's current usage of
// physical memory.
func memoryStats() (*memory, error) {
	mstat := struct {
		size      uint32
		_         uint32
		totalPhys uint64
		availPhys uint64
		_         [5]uint64
	}{}

	// windows sets return value to 0 when function fails.
	mstat.size = uint32(unsafe.Sizeof(mstat))
	ret, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mstat)))
	if ret == 0 {
		return nil, err
	}

	// memoryStats functions from other platforms return memory usage in bytes.
	return &memory{
		Usage: mstat.totalPhys - mstat.availPhys,
		Total: mstat.totalPhys,
	}, nil

}
