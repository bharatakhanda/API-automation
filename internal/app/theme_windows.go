//go:build windows

package app

import (
	"syscall"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

var (
	gdi32                = syscall.NewLazyDLL("gdi32.dll")
	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
)

type appTheme struct {
	windowBg win.HBRUSH
	inputBg  win.HBRUSH
}

func newAppTheme() appTheme {
	return appTheme{
		windowBg: solidBrush(win.RGB(246, 248, 251)),
		inputBg:  solidBrush(win.RGB(255, 255, 255)),
	}
}

func solidBrush(color win.COLORREF) win.HBRUSH {
	h, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return win.HBRUSH(h)
}

func (t appTheme) apply(m *MainWindow) {
	m.wnd.On().WmCtlColorDlg(func(p ui.WmCtlColor) win.HBRUSH { return t.windowBg })
	m.wnd.On().WmCtlColorStatic(func(p ui.WmCtlColor) win.HBRUSH {
		_, _ = p.Hdc().SetBkMode(co.BKMODE_TRANSPARENT)
		_, _ = p.Hdc().SetTextColor(win.RGB(23, 43, 77))
		return t.windowBg
	})
	m.wnd.On().WmCtlColorEdit(func(p ui.WmCtlColor) win.HBRUSH {
		_, _ = p.Hdc().SetBkColor(win.RGB(255, 255, 255))
		_, _ = p.Hdc().SetTextColor(win.RGB(23, 43, 77))
		return t.inputBg
	})
}
