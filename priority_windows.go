//go:build windows

package main

// SetProcessPriority is a no-op on Windows
// Windows priority is managed differently through the process handle
func SetProcessPriority(priority int) error {
	// On Windows, we would need to use Windows API to set process priority
	// For now, this is a no-op as it requires more complex implementation
	return nil
}
