//go:build windows

package main

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type processCPUTracker struct {
	mu       sync.Mutex
	lastWall time.Time
	lastCPU  uint64
}

func newProcessCPUTracker() *processCPUTracker {
	return &processCPUTracker{}
}

func (t *processCPUTracker) Sample() float64 {
	if t == nil {
		return 0
	}
	cpuTime, ok := currentProcessCPUTime()
	if !ok {
		return 0
	}
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lastWall.IsZero() || t.lastCPU == 0 {
		t.lastWall = now
		t.lastCPU = cpuTime
		return 0
	}
	wallDelta := now.Sub(t.lastWall)
	cpuDelta := cpuTime - t.lastCPU
	t.lastWall = now
	t.lastCPU = cpuTime
	if wallDelta <= 0 || cpuDelta == 0 {
		return 0
	}
	cpuSeconds := float64(cpuDelta) / 10_000_000
	percent := cpuSeconds / wallDelta.Seconds() * 100
	if cpus := runtime.NumCPU(); cpus > 0 {
		percent /= float64(cpus)
	}
	if percent < 0 {
		return 0
	}
	return percent
}

func currentProcessCPUTime() (uint64, bool) {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, false
	}
	var create, exit, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &create, &exit, &kernel, &user); err != nil {
		return 0, false
	}
	return filetimeToUint64(kernel) + filetimeToUint64(user), true
}

func filetimeToUint64(ft syscall.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

type processMemoryCountersEx struct {
	processMemoryCounters
	privateUsage uintptr
}

type processMemoryCountersEx2 struct {
	processMemoryCountersEx
	privateWorkingSetSize uintptr
	sharedCommitUsage     uintptr
}

var (
	psapiDLL                 = syscall.NewLazyDLL("psapi.dll")
	procGetProcessMemoryInfo = psapiDLL.NewProc("GetProcessMemoryInfo")
)

func currentProcessMemoryMB() float64 {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}

	// Prefer PrivateWorkingSetSize, which is closer to Task Manager's
	// per-process "Memory" display than total WorkingSetSize.
	countersEx2 := processMemoryCountersEx2{processMemoryCountersEx: processMemoryCountersEx{
		processMemoryCounters: processMemoryCounters{cb: uint32(unsafe.Sizeof(processMemoryCountersEx2{}))},
	}}
	if r1, _, _ := procGetProcessMemoryInfo.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&countersEx2)),
		uintptr(countersEx2.cb),
	); r1 != 0 {
		if countersEx2.privateWorkingSetSize > 0 {
			return float64(countersEx2.privateWorkingSetSize) / 1024 / 1024
		}
		if countersEx2.workingSetSize > 0 {
			return float64(countersEx2.workingSetSize) / 1024 / 1024
		}
		if countersEx2.privateUsage > 0 {
			return float64(countersEx2.privateUsage) / 1024 / 1024
		}
	}

	countersEx := processMemoryCountersEx{processMemoryCounters: processMemoryCounters{cb: uint32(unsafe.Sizeof(processMemoryCountersEx{}))}}
	if r1, _, _ := procGetProcessMemoryInfo.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&countersEx)),
		uintptr(countersEx.cb),
	); r1 != 0 {
		if countersEx.workingSetSize > 0 {
			return float64(countersEx.workingSetSize) / 1024 / 1024
		}
		if countersEx.privateUsage > 0 {
			return float64(countersEx.privateUsage) / 1024 / 1024
		}
	}

	counters := processMemoryCounters{cb: uint32(unsafe.Sizeof(processMemoryCounters{}))}
	if r1, _, _ := procGetProcessMemoryInfo.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.cb),
	); r1 == 0 {
		return 0
	}
	return float64(counters.workingSetSize) / 1024 / 1024
}
