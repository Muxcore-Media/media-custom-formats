package internal

import (
	"testing"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func TestSettingsSeedDefaults(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0", SeedDefaults: true})
	defs := m.Settings()
	if len(defs) < 1 || defs[0].Key != "seed_defaults" || defs[0].Type != contracts.SettingTypeBool {
		t.Fatalf("defs=%+v", defs)
	}
	if defs[0].Value != "true" {
		t.Fatalf("value=%q", defs[0].Value)
	}
	if err := m.UpdateSetting("seed_defaults", "false"); err != nil {
		t.Fatal(err)
	}
	if m.Settings()[0].Value != "false" {
		t.Fatalf("after update %q", m.Settings()[0].Value)
	}
}
