//go:build !windows

package profiles

import "syscall"

// pidAlive is os.kill(pid, 0) succeeding (any error counts as not alive, like Python's OSError).
func pidAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }
