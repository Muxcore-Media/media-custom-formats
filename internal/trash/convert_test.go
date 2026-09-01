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

func TestConvertYearEditionLanguage(t *testing.T) {
	yearCF := trash.CustomFormatJSON{
		TrashID: "cf-year", Name: "Year 1999", TrashScores: map[string]int{"default": 5},
		Specifications: []trash.Specification{{
			Name: "Year", Implementation: "YearSpecification", Required: true,
			Fields: []byte(`{"value":1999}`),
		}},
	}
	converted, skipped, warn := trash.ConvertFormat(yearCF, "radarr", "default")
	if skipped {
		t.Fatalf("year format skipped: %s", warn)
	}
	if len(converted.GetRules()) != 1 || converted.GetRules()[0].GetField() != "title" {
		t.Fatalf("year rules: %+v", converted.GetRules())
	}

	editionCF := trash.CustomFormatJSON{
		TrashID: "cf-edition", Name: "Director Cut", TrashScores: map[string]int{"default": 10},
		Specifications: []trash.Specification{{
			Name: "Edition", Implementation: "EditionSpecification", Required: true,
			Fields: []byte(`{"value":"Director.?Cut"}`),
		}},
	}
	converted, skipped, warn = trash.ConvertFormat(editionCF, "radarr", "default")
	if skipped {
		t.Fatalf("edition format skipped: %s", warn)
	}
	if converted.GetRules()[0].GetOp() != "matches" {
		t.Fatalf("edition rule: %+v", converted.GetRules()[0])
	}

	sizeCF := trash.CustomFormatJSON{
		TrashID: "cf-size", Name: "Big File", TrashScores: map[string]int{"default": 1},
		Specifications: []trash.Specification{{
			Name: "Size", Implementation: "SizeSpecification", Required: true,
			Fields: []byte(`{"min":10,"max":0}`),
		}},
	}
	converted, skipped, warn = trash.ConvertFormat(sizeCF, "radarr", "default")
	if skipped {
		t.Fatalf("size format skipped: %s", warn)
	}
	if converted.GetRules()[0].GetField() != "size" || converted.GetRules()[0].GetOp() != "gt" {
		t.Fatalf("size rule: %+v", converted.GetRules()[0])
	}

	langCF := trash.CustomFormatJSON{
		TrashID: "cf-lang", Name: "French", TrashScores: map[string]int{"default": 2},
		Specifications: []trash.Specification{{
			Name: "French", Implementation: "LanguageSpecification", Required: true,
			Fields: []byte(`{"value":2}`),
		}},
	}
	converted, skipped, warn = trash.ConvertFormat(langCF, "radarr", "default")
	if skipped {
		t.Fatalf("language format skipped: %s", warn)
	}
	if converted.GetRules()[0].GetField() != "title" {
		t.Fatalf("language rule: %+v", converted.GetRules()[0])
	}
}

func TestLoadQualityProfileItemsFixture(t *testing.T) {
	root := fixtureRoot(t)
	meta, err := trash.LoadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := trash.LoadQualityProfiles(root, meta, []string{"radarr"})
	if err != nil {
		t.Fatal(err)
	}
	var base *trash.QualityProfileJSON
	for _, item := range profiles {
		if item.Profile.Name == "Base Profile" {
			base = &item.Profile
			break
		}
	}
	if base == nil {
		t.Fatal("Base Profile not found in fixtures")
	}
	flat := trash.FlattenQualityItems(base.Items)
	if flat["CAM"] || !flat["Remux-2160p"] {
		t.Fatalf("unexpected quality items: CAM=%v Remux-2160p=%v", flat["CAM"], flat["Remux-2160p"])
	}
}
