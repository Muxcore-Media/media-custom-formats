package internal

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

type seedFormat struct {
	ID    string
	Name  string
	Score int32
	Rules []map[string]any
}

func (m *Module) seedDefaultFormats(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return
	}
	var n int
	if err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_formats`).Scan(&n); err != nil || n > 0 {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	seeds := []seedFormat{
		{ID: "cf_seed_remux", Name: "Remux", Score: 100, Rules: []map[string]any{{"field": "title", "op": "contains", "value": "Remux"}}},
		{ID: "cf_seed_hdr", Name: "HDR", Score: 50, Rules: []map[string]any{{"field": "title", "op": "matches", "value": `(?i)\bHDR(10|10\+|)?\b`}}},
		{ID: "cf_seed_x265", Name: "x265/HEVC", Score: 25, Rules: []map[string]any{{"field": "title", "op": "matches", "value": `(?i)x265|h\.?265|hevc`}}},
		{ID: "cf_seed_proper", Name: "Proper/Repack", Score: 20, Rules: []map[string]any{{"field": "title", "op": "matches", "value": `(?i)\b(proper|repack)\b`}}},
		{ID: "cf_seed_cam", Name: "CAM/TS", Score: -10000, Rules: []map[string]any{{"field": "title", "op": "matches", "value": `(?i)\b(cam|hdcam|telesync|hdts|tc)\b`}}},
	}
	for _, s := range seeds {
		raw, _ := json.Marshal(s.Rules)
		_, err := m.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO custom_formats (id, name, rules_json, default_score, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			s.ID, s.Name, string(raw), s.Score, now, now,
		)
		if err != nil {
			slog.Warn("seed format failed", "name", s.Name, "error", err)
		}
	}
	slog.Info("seeded default custom formats", "count", len(seeds))
}
