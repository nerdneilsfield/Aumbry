//go:build windows

package api

import "syscall"

func filetimeSeconds(ft syscall.Filetime) float64 {
	ticks := (uint64(ft.HighDateTime) << 32) | uint64(ft.LowDateTime)
	return float64(ticks) / 1e7
}

func readProcessCPUSeconds() float64 {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	var creation, exit, kernel, user syscall.Filetime
	if err = syscall.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return 0
	}
	return max(0.0, filetimeSeconds(kernel)+filetimeSeconds(user))
}
