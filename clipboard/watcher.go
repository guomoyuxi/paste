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
	interval      time.Duration
	lastTextHash  string
	lastImageHash string
	onChange      func(ClipChange)
	stopCh        chan struct{}
	mu            sync.Mutex
	running       bool
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
	// 同时检查图片和文本，独立判断各自是否有变化
	// 不能只检查图片就 return：某些应用复制文本时不会清除旧的图片 flavor，
	// 导致残留图片一直阻断文本检测
	imgData, imgErr := getClipboardImage()
	textData, textErr := getClipboardText()

	imgHash := ""
	if imgErr == nil && len(imgData) > 0 {
		h := sha256.Sum256(imgData)
		imgHash = hex.EncodeToString(h[:])
	}

	textHash := ""
	if textErr == nil && len(textData) > 0 {
		h := sha256.Sum256(textData)
		textHash = hex.EncodeToString(h[:])
	}

	imgChanged := imgHash != "" && imgHash != w.lastImageHash
	textChanged := textHash != "" && textHash != w.lastTextHash

	// 两者都变化时优先保存图片（图片常附带文本表示如 URL/文件名）
	if imgChanged {
		w.lastImageHash = imgHash
		w.lastTextHash = textHash // 同步更新，避免重复保存附带的文本
		if w.onChange != nil {
			w.onChange(ClipChange{
				Type:    "image",
				Content: imgData,
				Preview: fmt.Sprintf("图片 (%d bytes)", len(imgData)),
			})
		}
		return
	}

	if textChanged {
		w.lastTextHash = textHash
		w.lastImageHash = imgHash // 同步更新，避免残留图片 hash 导致后续误判
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
	// 优先尝试获取 PNG 数据 (截图通常是 PNG)
	if data, err := getClipboardDataAs("PNGf"); err == nil && len(data) > 0 {
		return data, nil
	}
	// PNGf 暂不可用，短暂等待后重试（osascript 设置剪切板时 TIFF 先于 PNG 就绪）
	time.Sleep(200 * time.Millisecond)
	if data, err := getClipboardDataAs("PNGf"); err == nil && len(data) > 0 {
		return data, nil
	}
	// 仍然没有 PNG，尝试获取 TIFF 数据并转换为 PNG
	if tiffData, err := getClipboardDataAs("TIFF"); err == nil && len(tiffData) > 0 {
		return convertTIFFToPNG(tiffData)
	}
	return nil, fmt.Errorf("no image in clipboard")
}

// getClipboardDataAs 从剪切板获取指定类型的数据，写入临时文件并读取
func getClipboardDataAs(class string) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "clip-*.dat")
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	os.Remove(tmpPath) // 删除空文件，让 osascript 创建新文件

	script := fmt.Sprintf(`
		try
			set theData to the clipboard as «class %s»
			set fh to open for access POSIX file "%s" with write permission
			set eof of fh to 0
			write theData to fh
			close access fh
			return "ok"
		on error errMsg
			return "error:" & errMsg
		end try
	`, class, tmpPath)

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		os.Remove(tmpPath)
		return nil, err
	}
	result := strings.TrimSpace(string(out))
	if strings.HasPrefix(result, "error:") {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("osascript: %s", result)
	}

	data, err := os.ReadFile(tmpPath)
	os.Remove(tmpPath)
	return data, err
}

// convertTIFFToPNG 将 TIFF 数据转换为 PNG 格式
func convertTIFFToPNG(tiffData []byte) ([]byte, error) {
	tmpTiff, err := os.CreateTemp("", "clip-*.tiff")
	if err != nil {
		return nil, err
	}
	tmpTiffPath := tmpTiff.Name()
	defer os.Remove(tmpTiffPath)
	if _, err := tmpTiff.Write(tiffData); err != nil {
		tmpTiff.Close()
		return nil, err
	}
	tmpTiff.Close()

	tmpPng, err := os.CreateTemp("", "clip-*.png")
	if err != nil {
		return nil, err
	}
	tmpPngPath := tmpPng.Name()
	tmpPng.Close()
	defer os.Remove(tmpPngPath)

	if err := exec.Command("sips", "-s", "format", "png", tmpTiffPath, "--out", tmpPngPath).Run(); err != nil {
		return nil, fmt.Errorf("tiff to png: %w", err)
	}
	return os.ReadFile(tmpPngPath)
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
