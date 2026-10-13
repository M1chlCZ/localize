//go:build windows

package store

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const lockfileExclusiveLock = 0x00000002

var (
	kernel32DLL      = syscall.NewLazyDLL("kernel32.dll")
	lockFileExProc   = kernel32DLL.NewProc("LockFileEx")
	unlockFileExProc = kernel32DLL.NewProc("UnlockFileEx")
)

func lockFile(handle *os.File) error {
	overlapped := &syscall.Overlapped{}
	result, _, callErr := lockFileExProc.Call(
		handle.Fd(),
		uintptr(lockfileExclusiveLock),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(overlapped)),
	)
	runtime.KeepAlive(overlapped)
	if result == 0 {
		return callErr
	}
	return nil
}

func unlockFile(handle *os.File) {
	overlapped := &syscall.Overlapped{}
	_, _, _ = unlockFileExProc.Call(
		handle.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(overlapped)),
	)
	runtime.KeepAlive(overlapped)
}
