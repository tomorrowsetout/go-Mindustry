//go:build !windows

package main

import "runtime"

type processCPUTracker struct{}

func newProcessCPUTracker() *processCPUTracker {
	return &processCPUTracker{}
}

func (t *processCPUTracker) Sample() float64 {
	return 0
}

func currentProcessMemoryMB() float64 {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return float64(mem.Sys) / 1024 / 1024
}
