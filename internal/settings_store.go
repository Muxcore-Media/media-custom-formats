package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Muxcore-Media/media-custom-formats/internal/trash"
	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

func (m *Module) migrateSettingsKV(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS settings_kv (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	return err
}

func (m *Module) persistSetting(key, value string) {
	m.mu.Lock()
	db := m.db
	m.mu.Unlock()
	if db == nil {
		return
	}
	_, _ = db.ExecContext(context.Background(),
		`INSERT INTO settings_kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
}

func (m *Module) loadPersistedSettings(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return
	}
	rows, err := m.db.QueryContext(ctx, `SELECT key, value FROM settings_kv`)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) != nil {
			continue
		}
		switch k {
		case "seed_defaults":
			if b, err := parseBool(v); err == nil {
				m.seedDefaults = b
			}
		case "trash_sync_on_start":
			if b, err := parseBool(v); err == nil {
				m.trashSyncOnStart = b
			}
		case "trash_guides_path":
			m.trashGuidesPath = strings.TrimSpace(v)
		case "trash_score_set":
			m.trashScoreSet = strings.TrimSpace(v)
			if m.trashScoreSet == "" {
				m.trashScoreSet = "default"
			}
		case "trash_import_profiles":
			if b, err := parseBool(v); err == nil {
				m.trashImportProfiles = b
			}
		case "trash_sync_interval_hours":
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
				m.trashSyncIntervalHours = n
			}
		}
	}
}

func qualityItemsFromTrash(items []trash.QualityProfileItem) []*qualityAllowItem {
	flat := trash.FlattenQualityItems(items)
	out := make([]*qualityAllowItem, 0, len(flat))
	for name, allowed := range flat {
		out = append(out, &qualityAllowItem{Name: name, Allowed: allowed})
	}
	return out
}

type qualityAllowItem struct {
	Name    string `json:"name"`
	Allowed bool   `json:"allowed"`
}

func encodeQualityItemsJSON(items []*qualityAllowItem) string {
	if len(items) == 0 {
		return "[]"
	}
	raw, _ := json.Marshal(items)
	return string(raw)
}

func decodeQualityItemsJSON(raw string) []*qualityAllowItem {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var items []*qualityAllowItem
	_ = json.Unmarshal([]byte(raw), &items)
	return items
}

func trashQualityName(q *formatsv1.QualityInfo) string {
	if q == nil {
		return "Unknown"
	}
	src := q.GetSource()
	res := q.GetResolution()
	switch src {
	case "CAM":
		return "CAM"
	case "BR-DISK":
		return "BR-DISK"
	case "RawHD":
		return "Raw-HD"
	case "DVD":
		return "DVD"
	case "Remux":
		if res != "" && res != "SD" {
			return "Remux-" + res
		}
		return "Remux-1080p"
	case "BluRay":
		if res != "" && res != "SD" {
			return "Bluray-" + res
		}
		return "Bluray-1080p"
	case "WEB-DL":
		if res != "" && res != "SD" {
			return "WEBDL-" + res
		}
		return "WEBDL-1080p"
	case "WEBRip":
		if res != "" && res != "SD" {
			return "WEBRip-" + res
		}
		return "WEBRip-1080p"
	case "HDTV":
		if res != "" && res != "SD" {
			return "HDTV-" + res
		}
		return "HDTV-1080p"
	default:
		return "Unknown"
	}
}

func qualityAllowedByProfile(qName string, items []*qualityAllowItem) (allowed bool, found bool) {
	if len(items) == 0 {
		return true, false
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Name, qName) {
			return item.Allowed, true
		}
	}
	return true, false
}
