//go:build linux || darwin

package main

import "syscall"

// SetProcessPriority sets the priority of the current process
// Returns nil if successful, error otherwise
func SetProcessPriority(priority int) error {
	return syscall.Setpriority(syscall.PRIO_PROCESS, 0, priority)
}
