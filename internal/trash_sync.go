package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Muxcore-Media/media-custom-formats/internal/trash"
	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

// SyncTrashGuides imports all convertible TRaSH Guides custom formats (and optional profiles).
func (m *Module) SyncTrashGuides(ctx context.Context, req *formatsv1.SyncTrashGuidesRequest) (*formatsv1.SyncTrashGuidesResponse, error) {
	path := strings.TrimSpace(req.GetPath())
	if path == "" {
		path = m.trashGuidesPath
	}
	scoreSet := strings.TrimSpace(req.GetScoreSet())
	if scoreSet == "" {
		scoreSet = m.trashScoreSet
	}
	if scoreSet == "" {
		scoreSet = "default"
	}
	importProfiles := req.GetImportProfiles() || m.trashImportProfiles
	services := req.GetServices()
	if len(services) == 0 {
		services = m.trashServices
	}

	root, err := trash.ResolveRoot(path, m.trashGuidesURL, m.trashCacheDir)
	if err != nil {
		return nil, err
	}
	meta, err := trash.LoadRoot(root)
	if err != nil {
		return nil, err
	}

	cfs, err := trash.LoadCustomFormats(root, meta, services)
	if err != nil {
		return nil, err
	}

	resp := &formatsv1.SyncTrashGuidesResponse{GuidesPath: root}
	scoreByTrashID := map[string]int32{}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, item := range cfs {
		converted, skipped, warn := trash.ConvertFormat(item.Format, item.Service, scoreSet)
		if skipped {
			resp.FormatsSkipped++
			if warn != "" && len(resp.Warnings) < 50 {
				resp.Warnings = append(resp.Warnings, fmt.Sprintf("%s/%s: %s", item.Service, item.Format.Name, warn))
			}
			continue
		}
		name, err := m.uniqueFormatName(converted.GetId(), converted.GetName(), item.Service)
		if err != nil {
			resp.FormatsSkipped++
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("%s: name conflict: %v", converted.GetName(), err))
			continue
		}
		converted.Name = name
		raw, _ := json.Marshal(converted.GetRules())
		_, err = m.db.ExecContext(ctx, `
			INSERT INTO custom_formats (id, name, rules_json, default_score, trash_id, trash_service, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name=excluded.name,
				rules_json=excluded.rules_json,
				default_score=excluded.default_score,
				trash_id=excluded.trash_id,
				trash_service=excluded.trash_service,
				updated_at=excluded.updated_at
		`, converted.GetId(), converted.GetName(), string(raw), converted.GetDefaultScore(),
			converted.GetTrashId(), converted.GetTrashService(), now, now)
		if err != nil {
			resp.FormatsSkipped++
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("%s upsert: %v", converted.GetName(), err))
			continue
		}
		resp.FormatsUpserted++
		scoreByTrashID[converted.GetId()] = converted.GetDefaultScore()
	}

	if importProfiles {
		profiles, err := trash.LoadQualityProfiles(root, meta, services)
		if err != nil {
			resp.Warnings = append(resp.Warnings, "quality profiles: "+err.Error())
		} else {
			for _, item := range profiles {
				qp := item.Profile
				if qp.TrashID == "" || qp.Name == "" {
					continue
				}
				formatScores := map[string]int32{}
				for _, trashID := range qp.FormatItems {
					if s, ok := scoreByTrashID[trashID]; ok {
						formatScores[trashID] = s
						continue
					}
					// Fall back to DB score if CF was imported earlier / other score set.
					if f := m.loadFormat(trashID); f != nil {
						formatScores[trashID] = f.GetDefaultScore()
					}
				}
				scoresJSON, _ := json.Marshal(formatScores)
				name, _ := m.uniqueProfileName(qp.TrashID, qp.Name, item.Service)
				_, err := m.db.ExecContext(ctx, `
					INSERT INTO quality_profiles (id, name, min_score, cutoff_score, upgrade_allowed, upgrade_delay_minutes, format_scores_json, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)
					ON CONFLICT(id) DO UPDATE SET
						name=excluded.name,
						min_score=excluded.min_score,
						cutoff_score=excluded.cutoff_score,
						upgrade_allowed=excluded.upgrade_allowed,
						format_scores_json=excluded.format_scores_json,
						updated_at=excluded.updated_at
				`, qp.TrashID, name, qp.MinFormatScore, qp.CutoffFormatScore, boolToInt(qp.UpgradeAllowed), string(scoresJSON), now, now)
				if err != nil {
					resp.Warnings = append(resp.Warnings, fmt.Sprintf("profile %s: %v", qp.Name, err))
					continue
				}
				resp.ProfilesUpserted++
			}
		}
	}

	slog.Info("trash guides sync complete",
		"path", root,
		"formats", resp.FormatsUpserted,
		"skipped", resp.FormatsSkipped,
		"profiles", resp.ProfilesUpserted,
		"score_set", scoreSet,
	)
	return resp, nil
}

func (m *Module) uniqueFormatName(id, name, service string) (string, error) {
	var existingID string
	err := m.db.QueryRow(`SELECT id FROM custom_formats WHERE name = ?`, name).Scan(&existingID)
	if err == nil && existingID != id {
		alt := fmt.Sprintf("%s (%s)", name, service)
		err2 := m.db.QueryRow(`SELECT id FROM custom_formats WHERE name = ?`, alt).Scan(&existingID)
		if err2 == nil && existingID != id {
			return fmt.Sprintf("%s (%s-%s)", name, service, id[:8]), nil
		}
		return alt, nil
	}
	return name, nil
}

func (m *Module) uniqueProfileName(id, name, service string) (string, error) {
	var existingID string
	err := m.db.QueryRow(`SELECT id FROM quality_profiles WHERE name = ?`, name).Scan(&existingID)
	if err == nil && existingID != id {
		return fmt.Sprintf("%s (%s)", name, service), nil
	}
	return name, nil
}

func (m *Module) maybeSyncTrashOnStart(ctx context.Context) bool {
	if !m.trashSyncOnStart {
		return false
	}
	resp, err := m.SyncTrashGuides(ctx, &formatsv1.SyncTrashGuidesRequest{
		Path:           m.trashGuidesPath,
		ScoreSet:       m.trashScoreSet,
		ImportProfiles: m.trashImportProfiles,
		Services:       m.trashServices,
	})
	if err != nil {
		slog.Warn("trash guides sync on start failed", "error", err)
		return false
	}
	slog.Info("trash guides synced on start", "formats", resp.GetFormatsUpserted(), "profiles", resp.GetProfilesUpserted())
	return resp.GetFormatsUpserted() > 0
}
