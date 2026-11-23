//go:build linux

package api

import (
	"os"
	"strconv"
	"strings"
)

func readProcessRSSBytes() uint64 {
	pageSize := os.Getpagesize()
	if pageSize <= 0 {
		return 0
	}
	raw, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * uint64(pageSize)
}
