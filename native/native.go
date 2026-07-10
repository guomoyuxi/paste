package native

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -I${SRCDIR}
#cgo LDFLAGS: -framework Cocoa -framework Carbon

#include "native.h"
*/
import "C"
import "log"

var (
	hotkeyCallback     func()
	menuItemCb         func(tag int)
	windowCloseCb      func()
)

//export hotkeyPressed
func hotkeyPressed() {
	if hotkeyCallback != nil {
		hotkeyCallback()
	}
}

//export menuItemClicked
func menuItemClicked(tag C.int) {
	log.Printf("[native] 菜单项点击: tag=%d", int(tag))
	if menuItemCb != nil {
		menuItemCb(int(tag))
	} else {
		log.Println("[native] menuItemCb 未设置")
	}
}

//export windowCloseRequested
func windowCloseRequested() {
	if windowCloseCb != nil {
		windowCloseCb()
	}
}

// CreateStatusBar 在当前 NSApplication 上创建状态栏图标和菜单
func CreateStatusBar() {
	C.cgoCreateStatusBar()
}

// RegisterHotKey 注册全局快捷键
func RegisterHotKey(keycode, modifiers int, callback func()) {
	hotkeyCallback = callback
	C.cgoRegisterHotKey(C.int(keycode), C.int(modifiers))
}

// UnregisterHotKey 注销全局快捷键
func UnregisterHotKey() {
	C.cgoUnregisterHotKey()
}

// SetMenuCallbacks 设置菜单项回调
func SetMenuCallbacks(showWindow, clearAll, quit func()) {
	menuItemCb = func(tag int) {
		switch tag {
		case 1:
			if showWindow != nil {
				showWindow()
			}
		case 2:
			if clearAll != nil {
				clearAll()
			}
		case 3:
			if quit != nil {
				quit()
			}
		}
	}
}

// HideApp 隐藏应用窗口
func HideApp() {
	C.cgoHideApp()
}

// UnhideApp 显示应用窗口并激活
func UnhideApp() {
	C.cgoUnhideApp()
}

// UnhideAppAsync 在主线程异步显示应用窗口（供非主线程调用）
func UnhideAppAsync() {
	C.cgoUnhideAppAsync()
}

// RegisterActivationObserver 注册应用激活观察者
// 当用户点击应用图标重新激活应用时，自动显示窗口
func RegisterActivationObserver() {
	C.cgoRegisterActivationObserver()
}

// SetupWindowDelegate 设置窗口关闭拦截，关闭时隐藏而非退出
func SetupWindowDelegate(onClose func()) {
	windowCloseCb = onClose
	C.cgoSetupWindowDelegate()
}

// macOS Carbon 修饰符常量
const (
	CmdKey     = 1 << 8  // cmdKey
	ShiftKey   = 1 << 9  // shiftKey
	OptionKey  = 1 << 11 // optionKey
	ControlKey = 1 << 12 // controlKey
)

// macOS 虚拟键码
const (
	KeyV     = 9  // kVK_ANSI_V
	KeyC     = 8  // kVK_ANSI_C
	KeySpace = 49 // kVK_Space
)
