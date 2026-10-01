package main

import (
	"runtime"
	"syscall"
	"unsafe"
)

// Minimal native Win32 progress window (status label + progress bar),
// built on raw syscalls since the launcher has no GUI library dependency.
// Needed because the launcher is -H windowsgui (no console), so extraction
// of the multi-GB disk image would otherwise look like a silent hang.

const (
	wsOverlapped = 0x00000000
	wsCaption    = 0x00C00000
	wsSysMenu    = 0x00080000
	wsVisible    = 0x10000000
	wsChild      = 0x40000000

	wmDestroy = 0x0002
	wmClose   = 0x0010

	pbmSetRange32 = 0x0406
	pbmSetPos     = 0x0402

	swShow     = 5
	smCxScreen = 0
	smCyScreen = 1

	colorBtnFaceBrush = 16 // (HBRUSH)(COLOR_BTNFACE+1), standard dialog background idiom
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procSendMessageW        = user32.NewProc("SendMessageW")
	procSetWindowTextW      = user32.NewProc("SetWindowTextW")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	procInitCommonControls  = comctl32.NewProc("InitCommonControls")

	wndProcCallback = syscall.NewCallback(progressWndProc)
)

// wndClassExW mirrors the Win32 WNDCLASSEXW struct layout.
type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

// point and msgT mirror the Win32 POINT and MSG structs.
type point struct{ x, y int32 }

type msgT struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

func progressWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

type progressWindow struct {
	hwnd      uintptr
	hwndBar   uintptr
	hwndLabel uintptr
	ready     chan struct{}
	quit      chan struct{}
}

// newProgressWindow creates and shows a small native progress window,
// running its message loop on a dedicated, locked OS thread (required:
// Win32 windows are thread-affine). The returned handle's SetProgress and
// Close methods are safe to call from any goroutine - they go through
// SendMessage, which Windows marshals across threads correctly.
func newProgressWindow(title string) *progressWindow {
	pw := &progressWindow{ready: make(chan struct{}), quit: make(chan struct{})}
	go pw.run(title)
	<-pw.ready
	return pw
}

func (pw *progressWindow) run(title string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(pw.quit)

	procInitCommonControls.Call()

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, _ := syscall.UTF16PtrFromString("LinuxLabProgressWindow")
	wc := wndClassExW{
		lpfnWndProc:   wndProcCallback,
		hInstance:     hInstance,
		hbrBackground: colorBtnFaceBrush,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	const width, height = 420, 110
	screenW, _, _ := procGetSystemMetrics.Call(smCxScreen)
	screenH, _, _ := procGetSystemMetrics.Call(smCyScreen)
	x := (int32(screenW) - width) / 2
	y := (int32(screenH) - height) / 2

	titlePtr, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(wsOverlapped|wsCaption|wsSysMenu|wsVisible),
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		0, 0, hInstance, 0,
	)
	pw.hwnd = hwnd

	labelClass, _ := syscall.UTF16PtrFromString("STATIC")
	labelText, _ := syscall.UTF16PtrFromString("Starting...")
	hwndLabel, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(labelClass)),
		uintptr(unsafe.Pointer(labelText)),
		uintptr(wsChild|wsVisible),
		20, 15, width-60, 20,
		hwnd, 0, hInstance, 0,
	)
	pw.hwndLabel = hwndLabel

	barClass, _ := syscall.UTF16PtrFromString("msctls_progress32")
	hwndBar, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(barClass)),
		0,
		uintptr(wsChild|wsVisible),
		20, 45, width-60, 24,
		hwnd, 0, hInstance, 0,
	)
	pw.hwndBar = hwndBar
	procSendMessageW.Call(hwndBar, pbmSetRange32, 0, 100)

	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)

	close(pw.ready)

	var m msgT
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// SetProgress updates the status text and progress bar position (0-100).
func (pw *progressWindow) SetProgress(percent int, status string) {
	if pw == nil || pw.hwndBar == 0 {
		return
	}
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}
	procSendMessageW.Call(pw.hwndBar, pbmSetPos, uintptr(percent), 0)
	if status != "" {
		if text, err := syscall.UTF16PtrFromString(status); err == nil {
			procSetWindowTextW.Call(pw.hwndLabel, uintptr(unsafe.Pointer(text)))
		}
	}
}

// Close destroys the progress window and waits for its message loop to
// exit. Safe to call more than once.
func (pw *progressWindow) Close() {
	if pw == nil || pw.hwnd == 0 {
		return
	}
	select {
	case <-pw.quit:
		return // already closed
	default:
	}
	procSendMessageW.Call(pw.hwnd, wmClose, 0, 0)
	<-pw.quit
}
