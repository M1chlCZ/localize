//go:build unix

package store

import (
	"os"
	"syscall"
)

func lockFile(handle *os.File) error {
	return syscall.Flock(int(handle.Fd()), syscall.LOCK_EX)
}

func unlockFile(handle *os.File) {
	_ = syscall.Flock(int(handle.Fd()), syscall.LOCK_UN)
}
