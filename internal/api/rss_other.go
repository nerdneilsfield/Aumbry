//go:build !linux

package api

func readProcessRSSBytes() uint64 {
	return 0
}
