#import <Cocoa/Cocoa.h>
#include <Carbon/Carbon.h>
#include "native.h"

// 文件日志，确保不依赖 stderr
static void debugLog(NSString *msg) {
    NSString *full = [NSString stringWithFormat:@"[%@] %@\n",
        [NSDate date].description, msg];
    const char *cstr = [full UTF8String];
    FILE *f = fopen("/tmp/paste_debug.log", "a");
    if (f) {
        fputs(cstr, f);
        fclose(f);
    }
    NSLog(@"%@", msg);
}

// ===== 全局快捷键 =====

static EventHotKeyRef g_hotKeyRef;

static OSStatus hotKeyHandler(EventHandlerCallRef nextHandler, EventRef theEvent, void *userData) {
    hotkeyPressed();
    return noErr;
}

void cgoRegisterHotKey(int keycode, int modifiers) {
    EventTypeSpec eventType;
    eventType.eventClass = kEventClassKeyboard;
    eventType.eventKind  = kEventHotKeyPressed;

    InstallApplicationEventHandler(NewEventHandlerUPP(hotKeyHandler), 1, &eventType, NULL, NULL);

    EventHotKeyID hotKeyID;
    hotKeyID.signature = 'pst!';
    hotKeyID.id = 1;

    RegisterEventHotKey(keycode, modifiers, hotKeyID, GetApplicationEventTarget(), 0, &g_hotKeyRef);
}

void cgoUnregisterHotKey() {
    if (g_hotKeyRef != NULL) {
        UnregisterEventHotKey(g_hotKeyRef);
        g_hotKeyRef = NULL;
    }
}

// ===== 应用委托 =====
// 防止最后一个窗口关闭时应用自动退出

@interface AppDelegate : NSObject <NSApplicationDelegate>
@end

@implementation AppDelegate
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
    return NO;
}
@end

static AppDelegate *g_appDelegate;

// ===== 状态栏 =====

@interface StatusMenuHandler : NSObject
- (void)showWindow:(id)sender;
- (void)clearHistory:(id)sender;
- (void)quitApp:(id)sender;
@end

@implementation StatusMenuHandler
- (BOOL)validateMenuItem:(NSMenuItem *)menuItem {
    return YES;
}
- (void)showWindow:(id)sender {
    debugLog(@"点击: 显示窗口");
    menuItemClicked(1);
    cgoUnhideApp();
}
- (void)clearHistory:(id)sender {
    debugLog(@"点击: 清空历史");
    menuItemClicked(2);
}
- (void)quitApp:(id)sender {
    debugLog(@"点击: 退出");
    menuItemClicked(3);
    [NSApp terminate:nil];
}
@end

static StatusMenuHandler *g_menuHandler;
static NSStatusItem *g_statusItem;

void cgoCreateStatusBar() {
    // 设置应用委托，防止窗口全部关闭时应用退出
    g_appDelegate = [[AppDelegate alloc] init];
    [NSApp setDelegate:g_appDelegate];

    g_menuHandler = [[StatusMenuHandler alloc] init];
    debugLog([NSString stringWithFormat:@"g_menuHandler = %@", g_menuHandler]);

    NSMenu *menu = [[NSMenu alloc] init];
    menu.autoenablesItems = NO;

    NSMenuItem *showItem = [[NSMenuItem alloc] initWithTitle:@"显示窗口"
                                                      action:@selector(showWindow:)
                                               keyEquivalent:@""];
    showItem.target = g_menuHandler;
    showItem.enabled = YES;
    [menu addItem:showItem];

    [menu addItem:[NSMenuItem separatorItem]];

    NSMenuItem *clearItem = [[NSMenuItem alloc] initWithTitle:@"清空历史"
                                                       action:@selector(clearHistory:)
                                                keyEquivalent:@""];
    clearItem.target = g_menuHandler;
    clearItem.enabled = YES;
    [menu addItem:clearItem];

    NSMenuItem *quitItem = [[NSMenuItem alloc] initWithTitle:@"退出"
                                                      action:@selector(quitApp:)
                                               keyEquivalent:@"q"];
    quitItem.target = g_menuHandler;
    quitItem.enabled = YES;
    [menu addItem:quitItem];

    g_statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
    g_statusItem.button.title = @"\U00002702";
    g_statusItem.menu = menu;
    debugLog(@"状态栏创建完成");
}

void cgoHideApp() {
    debugLog(@"cgoHideApp: 隐藏主窗口");
    // 只隐藏主窗口（标题为 "Paste - 剪切板管理" 的窗口）
    // 不能隐藏 webview 内部窗口（如 Item-0），否则会导致 webview 崩溃
    for (NSWindow *window in [NSApp windows]) {
        if ([window.title isEqualToString:@"Paste - 剪切板管理"]) {
            debugLog([NSString stringWithFormat:@"  隐藏主窗口"]);
            [window orderOut:nil];
            break;
        }
    }
}

void cgoUnhideApp() {
    debugLog([NSString stringWithFormat:@"cgoUnhideApp: 显示主窗口，窗口数=%lu", (unsigned long)[[NSApp windows] count]]);
    [NSApp activateIgnoringOtherApps:YES];
    // 只显示主窗口
    for (NSWindow *window in [NSApp windows]) {
        if ([window.title isEqualToString:@"Paste - 剪切板管理"]) {
            debugLog([NSString stringWithFormat:@"  显示主窗口: visible=%d", (int)window.visible]);
            [window makeKeyAndOrderFront:nil];
            break;
        }
    }
}

void cgoUnhideAppAsync() {
    dispatch_async(dispatch_get_main_queue(), ^{
        cgoUnhideApp();
    });
}

// ===== 应用重新激活处理 =====
// 当用户点击 Dock/Finder 中的应用图标时，显示窗口

void cgoRegisterActivationObserver() {
    [[NSNotificationCenter defaultCenter]
        addObserverForName:NSApplicationDidBecomeActiveNotification
                    object:nil
                     queue:[NSOperationQueue mainQueue]
                usingBlock:^(NSNotification *note) {
        debugLog(@"applicationDidBecomeActive: 显示窗口");
        cgoUnhideApp();
    }];
}

// ===== 窗口关闭拦截 =====
// 直接替换关闭按钮的 target/action，使其执行隐藏而非关闭
// 这是最可靠的方案：不让窗口真正关闭，避免 webview 内部状态损坏

@interface CloseButtonInterceptor : NSObject
@end

@implementation CloseButtonInterceptor
- (void)hideInsteadOfClose:(id)sender {
    debugLog(@"closeButton -> hideInsteadOfClose");
    cgoHideApp();
}
@end

static CloseButtonInterceptor *g_closeInterceptor;

void cgoSetupWindowDelegate() {
    g_closeInterceptor = [[CloseButtonInterceptor alloc] init];
    NSArray *windows = [NSApp windows];
    debugLog([NSString stringWithFormat:@"SetupWindowDelegate: 发现 %lu 个窗口", (unsigned long)windows.count]);
    for (NSWindow *window in windows) {
        debugLog([NSString stringWithFormat:@"  窗口 visible=%d title=%@", (int)window.visible, window.title]);
        // 替换关闭按钮的行为：点击关闭按钮时隐藏窗口而非关闭
        NSButton *closeBtn = [window standardWindowButton:NSWindowCloseButton];
        if (closeBtn) {
            closeBtn.target = g_closeInterceptor;
            closeBtn.action = @selector(hideInsteadOfClose:);
            debugLog([NSString stringWithFormat:@"  已替换关闭按钮: title=%@", window.title]);
        }
        // LSUIElement 应用没有 Dock 图标，最小化后窗口无法恢复，移除最小化按钮
        window.styleMask = window.styleMask & ~NSWindowStyleMaskMiniaturizable;
    }
}
