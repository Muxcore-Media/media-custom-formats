package internal

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"google.golang.org/grpc"
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
	m.mu.RUnlock()
	return []contracts.SettingDef{
		{
			Key:         "seed_defaults",
			Label:       "Seed Default Formats",
			Type:        contracts.SettingTypeBool,
			Default:     "true",
			Value:       strconv.FormatBool(seed),
			Description: "When true, seed Remux/HDR/x265/Proper/CAM formats and the Default Blocklist on init (and re-seed if toggled on at runtime)",
			Required:    false,
			Group:       "Defaults",
		},
		{
			Key:         "trash_sync",
			Label:       "Sync TRaSH Guides",
			Type:        contracts.SettingTypeBool,
			Default:     "false",
			Value:       os.Getenv("FORMATS_TRASH_SYNC"),
			Description: "When true on start, import bundled Recyclarr-compatible TRaSH format packs (or FORMATS_TRASH_GUIDES_PATH / FORMATS_TRASH_OFFICIAL)",
			Required:    false,
			Group:       "TRaSH",
		},
	}
}

func (m *Module) updateSetting(key, value string) error {
	switch key {
	case "trash_sync", "FORMATS_TRASH_SYNC":
		on, err := parseBool(value)
		if err != nil {
			return err
		}
		if on {
			_ = os.Setenv("FORMATS_TRASH_SYNC", "true")
			m.maybeSyncTrashOnStart(context.Background())
		} else {
			_ = os.Setenv("FORMATS_TRASH_SYNC", "false")
		}
		return nil
	case "seed_defaults", "FORMATS_SEED_DEFAULTS":
		on, err := parseBool(value)
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.seedDefaults = on
		db := m.db
		m.mu.Unlock()
		if on && db != nil {
			ctx := context.Background()
			m.seedDefaultFormats(ctx)
			m.seedDefaultReleaseGroups(ctx)
		}
		return nil
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
