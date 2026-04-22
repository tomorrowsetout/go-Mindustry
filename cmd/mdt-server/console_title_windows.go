//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

func setProcessConsoleTitle(title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("SetConsoleTitleW")
	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	_, _, _ = proc.Call(uintptr(unsafe.Pointer(ptr)))
}
