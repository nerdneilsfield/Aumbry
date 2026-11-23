//go:build !linux && !darwin && !windows

package api

func readProcessCPUSeconds() float64 {
	return 0
}
