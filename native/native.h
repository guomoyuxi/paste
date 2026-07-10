#ifndef PASTE_NATIVE_H
#define PASTE_NATIVE_H

extern void hotkeyPressed();
extern void menuItemClicked(int tag);
extern void windowCloseRequested();

void cgoRegisterHotKey(int keycode, int modifiers);
void cgoUnregisterHotKey();
void cgoCreateStatusBar();
void cgoHideApp();
void cgoUnhideApp();
void cgoUnhideAppAsync();
void cgoRegisterActivationObserver();
void cgoSetupWindowDelegate();

#endif
