//go:build !windows

package storage

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive, non-blocking lock held until f is closed.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
