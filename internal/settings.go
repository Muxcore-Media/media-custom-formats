package internal

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"google.golang.org/grpc"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

func (m *Module) Settings() []contracts.SettingDef {
	return m.settingsDefs()
}

func (m *Module) UpdateSetting(key, value string) error {
	return m.updateSetting(key, value)
}

func (m *Module) registerSettingsMesh(srv *grpc.Server) {
	modulesdk.RegisterSettings(srv, m.id, m)
}

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.mu.RLock()
	seed := m.seedDefaults
	syncOn := m.trashSyncOnStart
	path := m.trashGuidesPath
	scoreSet := m.trashScoreSet
	importProfiles := m.trashImportProfiles
	interval := m.trashSyncIntervalHours
	m.mu.RUnlock()
	return []contracts.SettingDef{
		{
			Key:         "seed_defaults",
			Label:       "Seed Default Formats",
			Type:        contracts.SettingTypeBool,
			Default:     "true",
			Value:       strconv.FormatBool(seed),
			Description: "When true, seed Remux/HDR/x265/Proper/CAM formats and the Default Blocklist on init (skipped for formats when trash sync-on-start is enabled)",
			Required:    false,
			Group:       "Defaults",
		},
		{
			Key:         "trash_sync_on_start",
			Label:       "Sync TRaSH Guides On Start",
			Type:        contracts.SettingTypeBool,
			Default:     "false",
			Value:       strconv.FormatBool(syncOn),
			Description: "When true, import all TRaSH Guides custom formats at startup (FORMATS_TRASH_SYNC)",
			Required:    false,
			Group:       "TRaSH Guides",
		},
		{
			Key:         "trash_guides_path",
			Label:       "TRaSH Guides Path",
			Type:        contracts.SettingTypeString,
			Default:     "",
			Value:       path,
			Description: "Local path to a TRaSH Guides checkout (metadata.json). Empty downloads the official archive into the cache dir",
			Required:    false,
			Group:       "TRaSH Guides",
		},
		{
			Key:         "trash_score_set",
			Label:       "TRaSH Score Set",
			Type:        contracts.SettingTypeString,
			Default:     "default",
			Value:       scoreSet,
			Description: "Which trash_scores key to apply (default, german, anime-radarr, …)",
			Required:    false,
			Group:       "TRaSH Guides",
		},
		{
			Key:         "trash_import_profiles",
			Label:       "Import TRaSH Quality Profiles",
			Type:        contracts.SettingTypeBool,
			Default:     "false",
			Value:       strconv.FormatBool(importProfiles),
			Description: "Also upsert TRaSH quality profiles when syncing",
			Required:    false,
			Group:       "TRaSH Guides",
		},
		{
			Key:         "trash_sync_interval_hours",
			Label:       "TRaSH Sync Interval (hours)",
			Type:        contracts.SettingTypeString,
			Default:     "0",
			Value:       strconv.Itoa(interval),
			Description: "When >0, periodically sync TRaSH Guides (FORMATS_TRASH_INTERVAL). 0 disables the in-process scheduler.",
			Required:    false,
			Group:       "TRaSH Guides",
		},
		{
			Key:         "trash_sync_now",
			Label:       "Sync TRaSH Guides Now",
			Type:        contracts.SettingTypeString,
			Default:     "",
			Value:       "",
			Description: "Set to true/1 to trigger an immediate SyncTrashGuides",
			Required:    false,
			Group:       "TRaSH Guides",
		},
	}
}

func (m *Module) updateSetting(key, value string) error {
	switch key {
	case "seed_defaults", "FORMATS_SEED_DEFAULTS":
		on, err := parseBool(value)
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.seedDefaults = on
		db := m.db
		syncOn := m.trashSyncOnStart
		m.mu.Unlock()
		m.persistSetting("seed_defaults", strconv.FormatBool(on))
		if on && db != nil {
			ctx := context.Background()
			if !syncOn {
				m.seedDefaultFormats(ctx)
			}
			m.seedDefaultReleaseGroups(ctx)
		}
		return nil
	case "trash_sync_on_start", "FORMATS_TRASH_SYNC":
		on, err := parseBool(value)
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.trashSyncOnStart = on
		m.mu.Unlock()
		m.persistSetting("trash_sync_on_start", strconv.FormatBool(on))
		return nil
	case "trash_guides_path", "FORMATS_TRASH_GUIDES_PATH":
		v := strings.TrimSpace(value)
		m.mu.Lock()
		m.trashGuidesPath = v
		m.mu.Unlock()
		m.persistSetting("trash_guides_path", v)
		return nil
	case "trash_score_set", "FORMATS_TRASH_SCORE_SET":
		m.mu.Lock()
		m.trashScoreSet = strings.TrimSpace(value)
		if m.trashScoreSet == "" {
			m.trashScoreSet = "default"
		}
		scoreSet := m.trashScoreSet
		m.mu.Unlock()
		m.persistSetting("trash_score_set", scoreSet)
		return nil
	case "trash_import_profiles", "FORMATS_TRASH_IMPORT_PROFILES":
		on, err := parseBool(value)
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.trashImportProfiles = on
		m.mu.Unlock()
		m.persistSetting("trash_import_profiles", strconv.FormatBool(on))
		return nil
	case "trash_sync_interval_hours", "FORMATS_TRASH_INTERVAL":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 {
			return fmt.Errorf("invalid interval %q", value)
		}
		m.mu.Lock()
		m.trashSyncIntervalHours = n
		m.mu.Unlock()
		m.persistSetting("trash_sync_interval_hours", strconv.Itoa(n))
		m.startTrashSyncTicker()
		return nil
	case "trash_sync_now":
		on, err := parseBool(value)
		if err != nil {
			return err
		}
		if !on {
			return nil
		}
		_, err = m.SyncTrashGuides(context.Background(), &formatsv1.SyncTrashGuidesRequest{})
		return err
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", value)
	}
}
