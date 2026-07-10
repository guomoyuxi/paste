package clipboard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type ClipChange struct {
	Type    string // "text" or "image"
	Content []byte
	Preview string
}

type Watcher struct {
	interval   time.Duration
	lastTextHash string
	lastImageHash string
	onChange    func(ClipChange)
	stopCh     chan struct{}
	mu         sync.Mutex
	running    bool
}

func NewWatcher(interval time.Duration, onChange func(ClipChange)) *Watcher {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &Watcher{
		interval: interval,
		onChange: onChange,
		stopCh:   make(chan struct{}),
	}
}

func (w *Watcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	go w.loop()
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running {
		return
	}
	w.running = false
	close(w.stopCh)
}

func (w *Watcher) loop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.checkClipboard()
		}
	}
}

func (w *Watcher) checkClipboard() {
	// 检查图片(优先，因为截图通常是图片)
	if imgData, err := getClipboardImage(); err == nil && len(imgData) > 0 {
		hash := sha256.Sum256(imgData)
		hashStr := hex.EncodeToString(hash[:])
		if hashStr != w.lastImageHash {
			w.lastImageHash = hashStr
			// 同时清除文本hash，因为图片复制也可能改变文本剪切板
			w.lastTextHash = ""
			if w.onChange != nil {
				w.onChange(ClipChange{
					Type:    "image",
					Content: imgData,
					Preview: fmt.Sprintf("图片 (%d bytes)", len(imgData)),
				})
			}
		}
		return
	}

	// 检查文本
	if textData, err := getClipboardText(); err == nil && len(textData) > 0 {
		hash := sha256.Sum256(textData)
		hashStr := hex.EncodeToString(hash[:])
		if hashStr != w.lastTextHash {
			w.lastTextHash = hashStr
			w.lastImageHash = ""
			preview := string(textData)
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			if w.onChange != nil {
				w.onChange(ClipChange{
					Type:    "text",
					Content: textData,
					Preview: preview,
				})
			}
		}
	}
}

func getClipboardText() ([]byte, error) {
	// 使用 osascript 获取剪切板文本，保证 UTF-8 编码
	// pbpaste 在某些环境下可能返回非 UTF-8 编码（如 MacRoman），导致中文乱码
	script := `
		try
			set theText to the clipboard as text
			return theText
		on error
			return ""
		end try
	`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return nil, err
	}
	// osascript 返回末尾带换行，去掉
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty clipboard")
	}
	return out, nil
}

func getClipboardImage() ([]byte, error) {
	// 使用 osascript 检查剪切板是否包含图片，然后用 png格式获取
	script := `
		try
			set theClip to the clipboard as «class PNGf»
			return "has_image"
		on error
			return "no_image"
		end try
	`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil || string(out) != "has_image\n" {
		return nil, fmt.Errorf("no image in clipboard")
	}

	// 使用 osascript 将剪切板图片写入临时文件
	script2 := `
		set tmpFile to (POSIX path of (path to temporary items)) & "clipboard_img.png"
		try
			set imgData to the clipboard as «class PNGf»
			set fh to open for access tmpFile with write permission
			write imgData to fh
			close access fh
			return tmpFile
		on error errMsg
			return "error:" & errMsg
		end try
	`
	out, err = exec.Command("osascript", "-e", script2).Output()
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(string(out))
	if strings.HasPrefix(path, "error:") {
		return nil, fmt.Errorf("osascript: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	os.Remove(path)
	return data, nil
}

// PasteToClipboard 将内容写回系统剪切板
func PasteToClipboard(contentType string, data []byte) error {
	if contentType == "text" {
		// 使用 osascript 设置文本，保证 UTF-8 编码
		// pbcopy 在某些环境下也可能有编码问题
		tmpFile, err := os.CreateTemp("", "paste-*.txt")
		if err != nil {
			return err
		}
		defer os.Remove(tmpFile.Name())
		tmpFile.Write(data)
		tmpFile.Close()

		script := fmt.Sprintf(`
			set theFile to POSIX file "%s"
			set theText to read theFile as «class utf8»
			set the clipboard to theText
		`, tmpFile.Name())
		return exec.Command("osascript", "-e", script).Run()
	}

	if contentType == "image" {
		// 写入临时文件再用 osascript 设置图片到剪切板
		tmpFile, err := os.CreateTemp("", "paste-*.png")
		if err != nil {
			return err
		}
		defer os.Remove(tmpFile.Name())
		tmpFile.Write(data)
		tmpFile.Close()

		script := fmt.Sprintf(`
			set theFile to POSIX file "%s"
			set imgData to read theFile as «class PNGf»
			set the clipboard to imgData
		`, tmpFile.Name())
		return exec.Command("osascript", "-e", script).Run()
	}

	return fmt.Errorf("unsupported content type: %s", contentType)
}
