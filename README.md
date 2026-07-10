# Paste - 剪切板管理

macOS 原生剪切板管理工具，支持文本和图片历史记录、全局快捷键、状态栏驻留。

## 功能特性

- **自动记录剪切板** — 实时监听系统剪切板，自动保存文本和图片
- **分类筛选** — 按全部/文本/图片/收藏快速筛选
- **全局快捷键** — `⌘⇧V` 随时呼出或隐藏窗口
- **状态栏驻留** — 关闭窗口后应用驻留在状态栏，不占用 Dock 位置
- **收藏管理** — 重要内容可收藏，永不过期
- **保留时长** — 支持 1小时/1天/7天/30天/永久 五种保留策略
- **搜索** — 实时搜索剪切板历史
- **图片预览** — 双击图片可全屏预览
- **暗色主题** — 软蓝灰色调，长时间使用不疲劳

## 系统要求

- macOS 10.13 或更高版本
- Intel 芯片或 Apple Silicon (M1/M2/M3/M4)

## 安装

1. 下载对应架构的 DMG 安装包：
   - Intel 芯片：`Paste-1.0.0-intel.dmg`
   - Apple Silicon：`Paste-1.0.0-arm64.dmg`
2. 打开 DMG，将 Paste 拖入 Applications 文件夹
3. 首次启动需在系统设置中授予辅助功能权限（用于全局快捷键）

### 提示"已损坏，无法打开"怎么办？

由于应用未进行苹果开发者签名，macOS Gatekeeper 可能会标记为"已损坏"。解决方法：

**方法一（推荐）：** 打开终端，执行以下命令：
```bash
xattr -cr /Applications/Paste.app
```

**方法二：** 在 Finder 中右键 Paste.app → 选择"打开" → 在弹窗中点击"打开"

## 使用

| 操作 | 说明 |
|------|------|
| `⌘⇧V` | 全局快捷键，呼出/隐藏窗口 |
| 点击列表项 | 复制到剪切板 |
| 右键列表项 | 复制/收藏/删除 |
| 双击图片 | 全屏预览 |
| 关闭窗口 | 隐藏到状态栏（应用继续运行） |
| 状态栏菜单 | 显示窗口/清空历史/退出 |

## 从源码构建

### 依赖

- Go 1.21+
- Xcode Command Line Tools（提供 clang）

### 构建

```bash
# 构建两个架构的 DMG
make release

# 仅构建 Intel 版
make amd64

# 仅构建 Apple Silicon 版
make arm64

# 开发模式运行
make run
```

构建产物位于 `build/` 目录：
- `Paste-1.0.0-intel.dmg` — Intel 安装包
- `Paste-1.0.0-arm64.dmg` — Apple Silicon 安装包

## 技术架构

- **后端**：Go + SQLite
- **前端**：HTML/CSS/JavaScript（原生，无框架）
- **窗口**：WKWebView（系统原生，无需安装 Chrome）
- **原生集成**：CGO + Objective-C（状态栏、全局快捷键、窗口管理）
- **单实例**：文件锁 + HTTP 激活机制

## 项目结构

```
.
├── main.go              # 应用入口
├── native/              # macOS 原生集成（Objective-C）
│   ├── native.m         # 状态栏、快捷键、窗口管理
│   ├── native.go        # Go 绑定
│   └── native.h         # 头文件
├── clipboard/           # 剪切板监听
│   └── watcher.go       # 轮询监听 + osascript 读写
├── storage/             # 数据存储
│   └── sqlite.go        # SQLite CRUD + 过期清理
├── handler/             # HTTP API
│   └── api.go           # RESTful 接口
├── frontend/            # 前端界面
│   ├── index.html
│   ├── app.js
│   └── style.css
├── assets/              # 应用图标
│   └── AppIcon.icns
├── Info.plist           # 应用配置
└── Makefile             # 构建脚本
```

## 数据存储

- 数据库：`~/Library/Application Support/Paste/paste.db`
- 配置目录：`~/Library/Application Support/Paste/`

## 许可证

MIT
