package main

import (
	"syscall"
	"unsafe"
)

// showErrorDialog displays a native Windows message box with the given
// text. Needed because the launcher is built with -H windowsgui (no
// console), so a plain fprintln to stderr would never be seen by the
// student if something goes wrong before/instead of RVVM's own window
// appearing.
func showErrorDialog(err error) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBoxW := user32.NewProc("MessageBoxW")

	title, _ := syscall.UTF16PtrFromString("Linux Lab - Error")
	text, e := syscall.UTF16PtrFromString(err.Error())
	if e != nil {
		return
	}

	const (
		mbOK          = 0x00000000
		mbIconError   = 0x00000010
		mbSystemModal = 0x00001000
	)
	messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(title)),
		uintptr(mbOK|mbIconError|mbSystemModal),
	)
}
