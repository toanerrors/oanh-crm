//go:build unix

package sqlite

import (
	"fmt"
	"os"
	"syscall"
)

func lockRoot(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("dataset already owned by another process: %w", err)
	}
	return f, nil
}
