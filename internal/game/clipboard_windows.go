//go:build windows

package game

import (
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procOpenClipboard  = user32.NewProc("OpenClipboard")
	procCloseClipboard = user32.NewProc("CloseClipboard")
	procEmptyClipboard = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc   = kernel32.NewProc("GlobalAlloc")
	procGlobalLock    = kernel32.NewProc("GlobalLock")
	procGlobalUnlock  = kernel32.NewProc("GlobalUnlock")
)

const (
	gmEM_MOVEABLE = 0x0002
	cfUNICODETEXT = 13
)

func setClipboardText(text string) error {
	if text == "" {
		text = " "
	}

	u16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	bytes := uintptr(len(u16) * 2)

	r1, _, err := procOpenClipboard.Call(0)
	if r1 == 0 {
		return err
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	h, _, err := procGlobalAlloc.Call(gmEM_MOVEABLE, bytes)
	if h == 0 {
		return err
	}

	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return err
	}

	mem := unsafe.Slice((*byte)(unsafe.Pointer(p)), bytes)
	copy(mem, unsafe.Slice((*byte)(unsafe.Pointer(&u16[0])), bytes))

	procGlobalUnlock.Call(h)

	r1, _, err = procSetClipboardData.Call(cfUNICODETEXT, h)
	if r1 == 0 {
		return err
	}

	return nil
}
