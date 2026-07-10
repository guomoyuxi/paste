package handler

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"paste/clipboard"
	"paste/storage"
)

type APIHandler struct {
	store *storage.Store
}

func NewAPIHandler(store *storage.Store) *APIHandler {
	return &APIHandler{store: store}
}

func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/items", h.handleItems)
	mux.HandleFunc("/api/items/", h.handleItemDetail)
	mux.HandleFunc("/api/settings", h.handleSettings)
	mux.HandleFunc("/api/paste/", h.handlePaste)
}

func (h *APIHandler) handleItems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		filterType := r.URL.Query().Get("type")
		items, err := h.store.List(filterType, 500)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, items)

	case http.MethodDelete:
		// ?favorites=false 表示保留收藏项，只清空非收藏
		// 不带参数或 ?favorites=true 表示清空全部
		keepFavorites := r.URL.Query().Get("favorites") == "false"
		if err := h.store.DeleteAll(keepFavorites); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *APIHandler) handleItemDetail(w http.ResponseWriter, r *http.Request) {
	suffix := r.URL.Path[len("/api/items/"):]

	// 检查是否是 favorite 操作: /api/items/{id}/favorite
	if strings.HasSuffix(suffix, "/favorite") {
		idStr := suffix[:len(suffix)-len("/favorite")]
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		if r.Method == http.MethodPost {
			isFav, err := h.store.ToggleFavorite(id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"isFavorite": isFav})
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 普通条目操作: /api/items/{id}
	id, err := strconv.ParseInt(suffix, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := h.store.Get(id)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "not found")
			} else {
				writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		// 返回完整内容
		result := map[string]interface{}{
			"id":         item.ID,
			"type":       item.Type,
			"isFavorite": item.IsFavorite,
			"retention":  item.Retention,
			"createdAt":  item.CreatedAt,
			"expiresAt":  item.ExpiresAt,
		}
		if item.Type == "text" {
			result["content"] = string(item.Content)
		} else {
			result["content"] = base64.StdEncoding.EncodeToString(item.Content)
		}
		writeJSON(w, http.StatusOK, result)

	case http.MethodDelete:
		if err := h.store.Delete(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *APIHandler) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := h.store.GetSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, settings)

	case http.MethodPut:
		var settings storage.Settings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := h.store.UpdateSettings(&settings); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *APIHandler) handlePaste(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.URL.Path[len("/api/paste/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	item, err := h.store.Get(id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	if err := clipboard.PasteToClipboard(item.Type, item.Content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleClipboardChange 处理剪切板变化事件，保存新内容
func (h *APIHandler) HandleClipboardChange(change clipboard.ClipChange) {
	settings, _ := h.store.GetSettings()
	if settings == nil {
		settings = &storage.Settings{DefaultRetention: "7d"}
	}

	item := &storage.ClipItem{
		Type:       change.Type,
		Content:    change.Content,
		Preview:    change.Preview,
		IsFavorite: false,
		Retention:  settings.DefaultRetention,
	}

	// 图片的 preview 存 base64 缩略图
	if change.Type == "image" {
		item.Preview = base64.StdEncoding.EncodeToString(change.Content)
	}

	id, err := h.store.Insert(item)
	if err != nil {
		log.Printf("保存剪切板内容失败: %v", err)
		return
	}
	log.Printf("保存剪切板内容: id=%d type=%s", id, change.Type)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
