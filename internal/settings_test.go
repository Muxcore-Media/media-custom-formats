package internal

import (
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
