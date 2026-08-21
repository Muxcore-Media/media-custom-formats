package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type releaseProfileGroup struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Preferred      []string `json:"preferred"`
	MustContain    []string `json:"must_contain"`
	MustNotContain []string `json:"must_not_contain"`
	PreferredScore int32    `json:"preferred_score"`
	Enabled        bool     `json:"enabled"`
}

func (m *Module) migrateReleaseGroups(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS release_profile_groups (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			data_json  TEXT NOT NULL DEFAULT '{}',
			enabled    INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)
	return err
}

func (m *Module) loadReleaseGroups() []releaseProfileGroup {
	if m.db == nil {
		return nil
	}
	rows, err := m.db.Query(`SELECT id, name, data_json, enabled FROM release_profile_groups`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []releaseProfileGroup
	for rows.Next() {
		var id, name, raw string
		var enabled int
		if err := rows.Scan(&id, &name, &raw, &enabled); err != nil {
			continue
		}
		g := releaseProfileGroup{ID: id, Name: name, Enabled: enabled != 0, PreferredScore: 10}
		_ = json.Unmarshal([]byte(raw), &g)
		g.ID = id
		g.Name = name
		g.Enabled = enabled != 0
		out = append(out, g)
	}
	return out
}

func applyReleaseGroups(title string, groups []releaseProfileGroup, score int32) (int32, bool) {
	lower := strings.ToLower(title)
	for _, g := range groups {
		if !g.Enabled {
			continue
		}
		for _, term := range g.MustNotContain {
			term = strings.TrimSpace(term)
			if term != "" && strings.Contains(lower, strings.ToLower(term)) {
				return -100000, false // rejected
			}
		}
		for _, term := range g.MustContain {
			term = strings.TrimSpace(term)
			if term != "" && !strings.Contains(lower, strings.ToLower(term)) {
				return -100000, false
			}
		}
		bonus := g.PreferredScore
		if bonus == 0 {
			bonus = 10
		}
		for _, term := range g.Preferred {
			term = strings.TrimSpace(term)
			if term != "" && strings.Contains(lower, strings.ToLower(term)) {
				score += bonus
			}
		}
	}
	return score, true
}

func (m *Module) upsertReleaseGroup(ctx context.Context, g releaseProfileGroup) error {
	if g.ID == "" {
		g.ID = fmt.Sprintf("rpg_%d", time.Now().UnixNano())
	}
	if g.Name == "" {
		return fmt.Errorf("name required")
	}
	raw, _ := json.Marshal(g)
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := 0
	if g.Enabled {
		enabled = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO release_profile_groups (id, name, data_json, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, data_json=excluded.data_json, enabled=excluded.enabled, updated_at=excluded.updated_at
	`, g.ID, g.Name, string(raw), enabled, now, now)
	return err
}

func (m *Module) seedDefaultReleaseGroups(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return
	}
	var n int
	if err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM release_profile_groups`).Scan(&n); err != nil || n > 0 {
		return
	}
	g := releaseProfileGroup{
		ID:             "rpg_seed_default",
		Name:           "Default Blocklist",
		MustNotContain: []string{"cam", "telesync", "hdcam"},
		Preferred:      []string{"bluray", "remux", "web-dl"},
		PreferredScore: 15,
		Enabled:        true,
	}
	raw, _ := json.Marshal(g)
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = m.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO release_profile_groups (id, name, data_json, enabled, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)`,
		g.ID, g.Name, string(raw), now, now,
	)
}
