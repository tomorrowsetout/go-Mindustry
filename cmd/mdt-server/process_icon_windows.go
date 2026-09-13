//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

func setProcessConsoleIcon(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	loadImage := user32.NewProc("LoadImageW")
	sendMessage := user32.NewProc("SendMessageW")

	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}

	const (
		imageIcon      = 1
		lrLoadFromFile = 0x0010
		lrDefaultSize  = 0x0040
		wmSetIcon      = 0x0080
		iconSmall      = 0
		iconBig        = 1
	)

	hIcon, _, _ := loadImage.Call(0, uintptr(unsafe.Pointer(ptr)), imageIcon, 0, 0, lrLoadFromFile|lrDefaultSize)
	if hIcon == 0 {
		return
	}
	_, _, _ = sendMessage.Call(hwnd, wmSetIcon, iconSmall, hIcon)
	_, _, _ = sendMessage.Call(hwnd, wmSetIcon, iconBig, hIcon)
}
