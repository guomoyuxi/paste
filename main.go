package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"paste/clipboard"
	"paste/handler"
	"paste/native"
	"paste/storage"

	"github.com/webview/webview_go"
)

//go:embed frontend/*
var frontendFS embed.FS

func main() {
	// 单实例检查: 使用文件锁防止多个实例
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("获取用户目录失败: %v", err)
	}
	dataDir := filepath.Join(homeDir, "Library", "Application Support", "Paste")
	if envDir := os.Getenv("PASTE_DATA_DIR"); envDir != "" {
		dataDir = envDir
	}
	lockPath := filepath.Join(dataDir, ".lock")
	portPath := filepath.Join(dataDir, ".port")
	pidPath := filepath.Join(dataDir, ".pid")
	os.MkdirAll(filepath.Dir(lockPath), 0755)
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err == nil {
		// 尝试获取独占锁
		err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err != nil {
			// 已有实例在运行，通过 HTTP 通知现有实例显示窗口
			log.Println("Paste 已在运行，尝试激活现有窗口")
			activated := false
			if portData, e := os.ReadFile(portPath); e == nil {
				portStr := strings.TrimSpace(string(portData))
				client := &http.Client{Timeout: 2 * time.Second}
				if resp, e := client.Post("http://127.0.0.1:"+portStr+"/api/activate", "", nil); e == nil {
					resp.Body.Close()
					activated = true
				}
			}
			if !activated {
				// 激活失败：旧实例可能已无响应，强制终止后重启
				log.Println("激活失败，终止无响应的旧实例")
				if pidData, e := os.ReadFile(pidPath); e == nil {
					var oldPid int
					if _, e := fmt.Sscanf(strings.TrimSpace(string(pidData)), "%d", &oldPid); e == nil && oldPid > 0 {
						syscall.Kill(oldPid, syscall.SIGKILL)
						// 等待旧进程退出，释放文件锁
						time.Sleep(500 * time.Millisecond)
					}
				}
				// 重试获取锁
				err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
				if err != nil {
					log.Println("仍无法获取锁，退出")
					os.Exit(0)
				}
			} else {
				os.Exit(0)
			}
		}
		// 锁成功，保持文件打开直到进程退出
		defer lockFile.Close()
	}
	os.MkdirAll(dataDir, 0755)

	// 写入当前进程 PID，供后续实例强制终止时使用
	os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove(pidPath)

	dbPath := filepath.Join(dataDir, "paste.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer store.Close()

	// 启动 HTTP 服务
	apiHandler := handler.NewAPIHandler(store)
	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux)

	// 嵌入的前端文件
	frontendSub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("加载前端资源失败: %v", err)
	}
	frontendServer := http.FileServer(http.FS(frontendSub))
	mux.Handle("/frontend/", http.StripPrefix("/frontend/", frontendServer))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("启动HTTP服务失败: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d/frontend/", port)

	// 写入端口文件，供第二个实例激活时使用
	os.WriteFile(portPath, []byte(strconv.Itoa(port)), 0644)
	defer os.Remove(portPath)

	// 激活端点：第二个实例通过 HTTP 调用此端点来显示窗口
	mux.HandleFunc("/api/activate", func(rw http.ResponseWriter, r *http.Request) {
		native.UnhideAppAsync()
		rw.Header().Set("Content-Type", "application/json")
		rw.Write([]byte(`{"status":"ok"}`))
	})

	go func() {
		if err := http.Serve(listener, mux); err != nil {
			log.Fatalf("HTTP服务错误: %v", err)
		}
	}()

	// 启动剪切板监听
	watcher := clipboard.NewWatcher(500*time.Millisecond, apiHandler.HandleClipboardChange)
	watcher.Start()
	defer watcher.Stop()

	// 启动过期清理
	go startCleanup(store)

	// 创建 webview 窗口
	w := webview.New(true)
	defer w.Destroy()

	w.SetTitle("Paste - 剪切板管理")
	w.SetSize(800, 600, webview.HintNone)

	// 创建状态栏(必须在 webview.New() 之后，Run() 之前)
	native.CreateStatusBar()

	// 设置窗口关闭回调：窗口关闭时应用驻留状态栏
	// 注意：窗口已通过 releasedWhenClosed=NO 保持不销毁，可后续重新显示
	native.SetupWindowDelegate(func() {
		log.Println("[main] 窗口已关闭，应用驻留状态栏")
	})

	// 注册应用激活观察者：用户点击应用图标时显示窗口
	native.RegisterActivationObserver()

	// 注册全局快捷键: Cmd+Shift+V
	native.RegisterHotKey(native.KeyV, native.CmdKey|native.ShiftKey, func() {
		native.UnhideApp()
	})

	// 设置菜单回调
	native.SetMenuCallbacks(
		func() { // 显示窗口
			log.Println("[main] 菜单: 显示窗口")
			native.UnhideApp()
		},
		func() { // 清空历史
			log.Println("[main] 菜单: 清空历史")
			store.DeleteAll(true) // 保留收藏
			log.Println("已清空历史(保留收藏)")
		},
		func() { // 退出
			log.Println("[main] 菜单: 退出")
			native.UnregisterHotKey()
			store.Close()
			os.Exit(0)
		},
	)

	// 绑定 Go 函数到 JS
	w.Bind("getAppInfo", func() map[string]string {
		return map[string]string{
			"name":    "Paste",
			"version": "1.0.0",
		}
	})

	w.Bind("hideWindow", func() {
		native.HideApp()
	})

	// 加载前端页面
	w.Navigate(url)

	// 信号处理
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		native.UnregisterHotKey()
		os.Exit(0)
	}()

	log.Printf("Paste 启动成功，访问 %s", url)
	log.Println("快捷键: Cmd+Shift+V 呼出窗口")
	log.Println("关闭窗口后应用驻留在状态栏")

	// 运行 webview 事件循环(阻塞)
	w.Run()

	// 清理
	native.UnregisterHotKey()
	log.Println("应用已退出")
}

func startCleanup(store *storage.Store) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		affected, err := store.CleanExpired()
		if err != nil {
			log.Printf("清理过期记录失败: %v", err)
		} else if affected > 0 {
			log.Printf("已清理 %d 条过期记录", affected)
		}
	}
}
