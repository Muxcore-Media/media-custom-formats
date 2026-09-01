package internal

import (
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func TestSettingsSeedDefaults(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0", SeedDefaults: true})
	defs := m.Settings()
	var seed *contracts.SettingDef
	for i := range defs {
		if defs[i].Key == "seed_defaults" {
			seed = &defs[i]
			break
		}
	}
	if seed == nil || seed.Type != contracts.SettingTypeBool {
		t.Fatalf("defs=%+v", defs)
	}
	if seed.Value != "true" {
		t.Fatalf("value=%q", seed.Value)
	}
	if err := m.UpdateSetting("seed_defaults", "false"); err != nil {
		t.Fatal(err)
	}
	defs = m.Settings()
	for _, d := range defs {
		if d.Key == "seed_defaults" && d.Value != "false" {
			t.Fatalf("after update %q", d.Value)
		}
	}
}

func TestSettingsPersistAcrossRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "formats.db")
	m := NewModule(Config{DBPath: dbPath, GRPCAddr: ":0", SeedDefaults: false})
	ctx := t.Context()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		key, val string
	}{
		{"trash_sync_on_start", "true"},
		{"trash_score_set", "german"},
		{"trash_import_profiles", "true"},
		{"trash_guides_path", "/tmp/guides"},
		{"seed_defaults", "false"},
	} {
		if err := m.UpdateSetting(tc.key, tc.val); err != nil {
			t.Fatalf("%s: %v", tc.key, err)
		}
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatal(err)
	}

	m2 := NewModule(Config{DBPath: dbPath, GRPCAddr: ":0", SeedDefaults: true, TrashScoreSet: "default"})
	if err := m2.Init(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m2.Stop(ctx) })

	if !m2.trashSyncOnStart || m2.trashScoreSet != "german" || !m2.trashImportProfiles ||
		m2.trashGuidesPath != "/tmp/guides" || m2.seedDefaults {
		t.Fatalf("restored settings: sync=%v score=%q import=%v path=%q seed=%v",
			m2.trashSyncOnStart, m2.trashScoreSet, m2.trashImportProfiles, m2.trashGuidesPath, m2.seedDefaults)
	}
}
