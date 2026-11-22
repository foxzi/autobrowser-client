//go:build windows

package profiles

import "os"

// pidAlive: FindProcess opens the process on Windows and fails when it does not exist.
func pidAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
