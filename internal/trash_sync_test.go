package internal

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

func trashFixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "testdata", "trash-guides"))
}

func TestSyncTrashGuidesFixtures(t *testing.T) {
	m := NewModule(Config{
		DBPath:       filepath.Join(t.TempDir(), "formats.db"),
		GRPCAddr:     ":0",
		SeedDefaults: false,
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	resp, err := m.SyncTrashGuides(ctx, &formatsv1.SyncTrashGuidesRequest{
		Path:           trashFixtureRoot(t),
		ScoreSet:       "default",
		ImportProfiles: true,
		Services:       []string{"radarr", "sonarr"},
	})
	if err != nil {
		t.Fatalf("SyncTrashGuides: %v", err)
	}
	if resp.GetFormatsUpserted() < 5 {
		t.Fatalf("formats upserted=%d skipped=%d warnings=%v", resp.GetFormatsUpserted(), resp.GetFormatsSkipped(), resp.GetWarnings())
	}
	if resp.GetProfilesUpserted() < 1 {
		t.Fatalf("expected at least one quality profile, got %d", resp.GetProfilesUpserted())
	}

	profiles, err := m.ListProfiles(ctx, &formatsv1.ListProfilesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var hasItems bool
	for _, p := range profiles.GetProfiles() {
		for _, item := range p.GetQualityItems() {
			if item.GetName() == "CAM" && !item.GetAllowed() {
				hasItems = true
				break
			}
		}
	}
	if !hasItems {
		t.Fatal("expected imported profile quality_items with CAM disallowed")
	}

	list, err := m.ListFormats(ctx, &formatsv1.ListFormatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var hdrID string
	for _, f := range list.GetFormats() {
		if f.GetTrashId() == "" {
			t.Fatalf("imported format missing trash_id: %s", f.GetName())
		}
		if f.GetName() == "HDR" || f.GetName() == "HDR (radarr)" {
			hdrID = f.GetId()
		}
	}
	if hdrID == "" {
		// may be disambiguated
		for _, f := range list.GetFormats() {
			if f.GetTrashService() == "radarr" && f.GetDefaultScore() == 500 {
				hdrID = f.GetId()
				break
			}
		}
	}
	if hdrID == "" {
		t.Fatal("HDR format not found after sync")
	}

	score, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title: "Movie.2024.2160p.BluRay.REMUX.HDR.mkv",
	})
	if err != nil {
		t.Fatal(err)
	}
	foundHDR := false
	for _, mtc := range score.GetFormatMatches() {
		if mtc.GetFormatId() == hdrID || mtc.GetFormatName() == "HDR" || mtc.GetFormatName() == "HDR (radarr)" {
			foundHDR = true
			if mtc.GetScore() < 400 {
				t.Fatalf("HDR score too low: %d", mtc.GetScore())
			}
		}
	}
	if !foundHDR {
		t.Fatalf("expected HDR match, matches=%v total=%d", score.GetFormatMatches(), score.GetTotalScore())
	}
}

func TestMatchFormatTrashRemuxTier(t *testing.T) {
	m := NewModule(Config{
		DBPath:       filepath.Join(t.TempDir(), "formats.db"),
		GRPCAddr:     ":0",
		SeedDefaults: false,
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	_, err := m.SyncTrashGuides(ctx, &formatsv1.SyncTrashGuidesRequest{
		Path:     trashFixtureRoot(t),
		Services: []string{"radarr"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Remux Tier 01 requires QualityModifier Remux + one of listed groups (e.g. FraMeSToR).
	hit, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title: "Movie.2024.2160p.BluRay.REMUX.HDR-FraMeSToR",
	})
	if err != nil {
		t.Fatal(err)
	}
	miss, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title: "Movie.2024.2160p.BluRay.REMUX.HDR-SomeOtherGroup",
	})
	if err != nil {
		t.Fatal(err)
	}
	var hitTier, missTier bool
	for _, mtc := range hit.GetFormatMatches() {
		if mtc.GetFormatName() == "Remux Tier 01" {
			hitTier = true
		}
	}
	for _, mtc := range miss.GetFormatMatches() {
		if mtc.GetFormatName() == "Remux Tier 01" {
			missTier = true
		}
	}
	if !hitTier {
		t.Fatalf("expected Remux Tier 01 on FraMeSToR release; matches=%v", hit.GetFormatMatches())
	}
	if missTier {
		t.Fatalf("did not expect Remux Tier 01 on unknown group; matches=%v", miss.GetFormatMatches())
	}
}
