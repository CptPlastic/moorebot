//go:build windows

package main

import (
	"os"
	"syscall"
)

func attachConsole() {
	const attachParent = ^uint32(0) // ATTACH_PARENT_PROCESS
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attach := kernel32.NewProc("AttachConsole")
	alloc := kernel32.NewProc("AllocConsole")
	r, _, _ := attach.Call(uintptr(attachParent))
	if r == 0 {
		alloc.Call()
	}
	out, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	errH, _ := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
	os.Stdout = os.NewFile(uintptr(out), "stdout")
	os.Stderr = os.NewFile(uintptr(errH), "stderr")
}
