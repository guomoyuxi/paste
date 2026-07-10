package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type ClipItem struct {
	ID          int64     `json:"id"`
	Type        string    `json:"type"` // "text" or "image"
	Content     []byte    `json:"-"`
	Preview     string    `json:"preview"`
	IsFavorite  bool      `json:"isFavorite"`
	Retention   string    `json:"retention"` // "1h","1d","7d","forever"
	CreatedAt   time.Time `json:"createdAt"`
	ExpiresAt   *time.Time `json:"expiresAt"`
}

type Settings struct {
	DefaultRetention string `json:"defaultRetention"`
}

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS clipboard_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			content BLOB NOT NULL,
			preview TEXT NOT NULL DEFAULT '',
			is_favorite INTEGER NOT NULL DEFAULT 0,
			retention TEXT NOT NULL DEFAULT '30d',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			expires_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_expires ON clipboard_items(expires_at);
		CREATE INDEX IF NOT EXISTS idx_favorite ON clipboard_items(is_favorite);
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		INSERT OR IGNORE INTO settings (key, value) VALUES ('defaultRetention', '30d');
	`)
	if err != nil {
		return err
	}
	// 迁移：将旧的本地时间转换为 UTC（仅执行一次）
	var migrated string
	err = s.db.QueryRow(`SELECT value FROM settings WHERE key = 'utc_migrated'`).Scan(&migrated)
	if err == sql.ErrNoRows {
		// 未迁移，将所有时间减去本地时区偏移
		_, offset := time.Now().Zone()
		hours := -(offset / 3600)
		_, err = s.db.Exec(`
			UPDATE clipboard_items SET 
				created_at = datetime(created_at, ?),
				expires_at = datetime(expires_at, ?)
		`, fmt.Sprintf("%+d hours", hours), fmt.Sprintf("%+d hours", hours))
		if err != nil {
			return err
		}
		s.db.Exec(`INSERT INTO settings (key, value) VALUES ('utc_migrated', '1')`)
	}
	return nil
}

func (s *Store) Insert(item *ClipItem) (int64, error) {
	now := time.Now().UTC()
	item.CreatedAt = now
	if item.Retention == "" {
		item.Retention = "30d"
	}
	if !item.IsFavorite {
		exp := calcExpiry(now, item.Retention)
		item.ExpiresAt = &exp
	}
	var expiresAt interface{}
	if item.ExpiresAt != nil {
		expiresAt = item.ExpiresAt.UTC().Format("2006-01-02 15:04:05")
	}
	res, err := s.db.Exec(
		`INSERT INTO clipboard_items (type, content, preview, is_favorite, retention, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		item.Type, item.Content, item.Preview, boolToInt(item.IsFavorite), item.Retention,
		now.Format("2006-01-02 15:04:05"), expiresAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) List(filterType string, limit int) ([]ClipItem, error) {
	if limit <= 0 {
		limit = 500
	}
	// 过滤已过期且未收藏的条目（收藏项 expires_at 为 NULL，天然不会被排除）
	query := `SELECT id, type, preview, is_favorite, retention, created_at, expires_at FROM clipboard_items WHERE (expires_at IS NULL OR expires_at >= datetime('now'))`
	args := []interface{}{}
	if filterType != "" {
		query += ` AND type = ?`
		args = append(args, filterType)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ClipItem
	for rows.Next() {
		var item ClipItem
		var isFav int
		var expiresAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Type, &item.Preview, &isFav, &item.Retention, &item.CreatedAt, &expiresAt); err != nil {
			return nil, err
		}
		item.IsFavorite = isFav == 1
		if expiresAt.Valid {
			item.ExpiresAt = &expiresAt.Time
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) Get(id int64) (*ClipItem, error) {
	var item ClipItem
	var isFav int
	var expiresAt sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, type, content, preview, is_favorite, retention, created_at, expires_at FROM clipboard_items WHERE id = ?`, id,
	).Scan(&item.ID, &item.Type, &item.Content, &item.Preview, &isFav, &item.Retention, &item.CreatedAt, &expiresAt)
	if err != nil {
		return nil, err
	}
	item.IsFavorite = isFav == 1
	if expiresAt.Valid {
		item.ExpiresAt = &expiresAt.Time
	}
	return &item, nil
}

func (s *Store) ToggleFavorite(id int64) (bool, error) {
	var isFav int
	var retention string
	err := s.db.QueryRow(`SELECT is_favorite, retention FROM clipboard_items WHERE id = ?`, id).Scan(&isFav, &retention)
	if err != nil {
		return false, err
	}
	newFav := 1 - isFav
	var expiresAt interface{}
	if newFav == 1 {
		expiresAt = nil // 收藏永不过期
	} else {
		exp := calcExpiry(time.Now().UTC(), retention)
		expiresAt = exp.UTC().Format("2006-01-02 15:04:05")
	}
	_, err = s.db.Exec(`UPDATE clipboard_items SET is_favorite = ?, expires_at = ? WHERE id = ?`, newFav, expiresAt, id)
	return newFav == 1, err
}

func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM clipboard_items WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteAll(keepFavorites bool) error {
	if keepFavorites {
		_, err := s.db.Exec(`DELETE FROM clipboard_items WHERE is_favorite = 0`)
		return err
	}
	_, err := s.db.Exec(`DELETE FROM clipboard_items`)
	return err
}

func (s *Store) GetFavorites() ([]ClipItem, error) {
	rows, err := s.db.Query(
		`SELECT id, type, preview, is_favorite, retention, created_at, expires_at FROM clipboard_items WHERE is_favorite = 1 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ClipItem
	for rows.Next() {
		var item ClipItem
		var isFav int
		var expiresAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Type, &item.Preview, &isFav, &item.Retention, &item.CreatedAt, &expiresAt); err != nil {
			return nil, err
		}
		item.IsFavorite = true
		if expiresAt.Valid {
			item.ExpiresAt = &expiresAt.Time
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) CleanExpired() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM clipboard_items WHERE expires_at IS NOT NULL AND expires_at < datetime('now') AND is_favorite = 0`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) GetSettings() (*Settings, error) {
	var val string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'defaultRetention'`).Scan(&val)
	if err != nil {
		return nil, err
	}
	return &Settings{DefaultRetention: val}, nil
}

func (s *Store) UpdateSettings(settings *Settings) error {
	_, err := s.db.Exec(`UPDATE settings SET value = ? WHERE key = 'defaultRetention'`, settings.DefaultRetention)
	return err
}

func calcExpiry(from time.Time, retention string) time.Time {
	switch retention {
	case "1h":
		return from.Add(1 * time.Hour)
	case "1d":
		return from.Add(24 * time.Hour)
	case "7d":
		return from.Add(7 * 24 * time.Hour)
	case "30d":
		return from.Add(30 * 24 * time.Hour)
	case "forever":
		return from.Add(365 * 24 * time.Hour) // 1年作为"永久"的实际过期时间
	default:
		return from.Add(30 * 24 * time.Hour)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
