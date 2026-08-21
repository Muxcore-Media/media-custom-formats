package trash_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Muxcore-Media/media-custom-formats/internal/trash"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "trash-guides")
	if _, err := trash.LoadRoot(root); err != nil {
		t.Fatalf("fixture root %s: %v", root, err)
	}
	return root
}

func TestLoadAndConvertFixtures(t *testing.T) {
	root := fixtureRoot(t)
	meta, err := trash.LoadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	cfs, err := trash.LoadCustomFormats(root, meta, []string{"radarr", "sonarr"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfs) < 5 {
		t.Fatalf("expected fixture CFs, got %d", len(cfs))
	}
	var hdrOK, remuxOK bool
	for _, item := range cfs {
		converted, skipped, warn := trash.ConvertFormat(item.Format, item.Service, "default")
		if skipped {
			t.Logf("skipped %s: %s", item.Format.Name, warn)
			continue
		}
		if converted.GetName() == "HDR" && item.Service == "radarr" {
			hdrOK = true
			if converted.GetDefaultScore() != 500 {
				t.Fatalf("HDR score: got %d", converted.GetDefaultScore())
			}
			if len(converted.GetRules()) == 0 {
				t.Fatal("HDR has no rules")
			}
		}
		if converted.GetName() == "Remux Tier 01" {
			remuxOK = true
			var hasRequired, hasOptional bool
			for _, r := range converted.GetRules() {
				if r.GetRequired() {
					hasRequired = true
				} else {
					hasOptional = true
				}
			}
			if !hasRequired || !hasOptional {
				t.Fatalf("Remux Tier 01 should have required + optional rules")
			}
		}
	}
	if !hdrOK || !remuxOK {
		t.Fatalf("missing expected formats hdrOK=%v remuxOK=%v", hdrOK, remuxOK)
	}
}
