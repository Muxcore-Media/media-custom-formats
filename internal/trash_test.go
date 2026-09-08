package internal

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

func TestParseTrashFormatRequiresAllTitleSpecs(t *testing.T) {
	raw, err := os.ReadFile("guides-fixture/docs/json/radarr/cf/remux-1080p.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := parseTrashFormat(raw, "default")
	if err != nil {
		t.Fatal(err)
	}
	if f.GetDefaultScore() != 1850 {
		t.Fatalf("score=%d", f.GetDefaultScore())
	}
	ok, _ := matchFormat(f, "Movie.2024.1080p.BluRay.REMUX.mkv", 0, 0)
	if !ok {
		t.Fatal("expected 1080p remux to match")
	}
	ok, _ = matchFormat(f, "Movie.2024.2160p.BluRay.REMUX.mkv", 0, 0)
	if ok {
		t.Fatal("2160p remux must not match Remux-1080p")
	}
}

func TestSyncTrashGuidesEmbedded(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()
	resp, err := m.SyncTrashGuides(ctx, &formatsv1.SyncTrashGuidesRequest{
		ScoreSet:       "default",
		ImportProfiles: true,
		Services:       []string{"radarr", "sonarr"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetFormatsUpserted() < 40 {
		t.Fatalf("formats upserted=%d want >= 40 (%s skipped=%d)", resp.GetFormatsUpserted(), resp.GetGuidesPath(), resp.GetFormatsSkipped())
	}
	if resp.GetProfilesUpserted() < 3 {
		t.Fatalf("profiles upserted=%d want >= 3", resp.GetProfilesUpserted())
	}
	list, err := m.ListFormats(ctx, &formatsv1.ListFormatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var remux bool
	for _, f := range list.GetFormats() {
		if f.GetName() == "Remux-1080p" && f.GetDefaultScore() == 1850 {
			remux = true
		}
	}
	if !remux {
		t.Fatal("expected Remux-1080p from bundled TRaSH pack")
	}
	profiles, err := m.ListProfiles(ctx, &formatsv1.ListProfilesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles.GetProfiles()) < 3 {
		t.Fatalf("profiles=%d", len(profiles.GetProfiles()))
	}
}

func TestSyncTrashGuidesFromPath(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()
	root, err := filepath.Abs("guides-fixture")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := m.SyncTrashGuides(ctx, &formatsv1.SyncTrashGuidesRequest{
		ImportProfiles: true,
		GuidesPath:     root,
		Services:       []string{"radarr"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetFormatsUpserted() < 30 {
		t.Fatalf("radarr formats=%d path=%s skipped=%d", resp.GetFormatsUpserted(), resp.GetGuidesPath(), resp.GetFormatsSkipped())
	}
	score, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title: "Dune.2021.1080p.BluRay.REMUX.HDR.mkv",
	})
	if err != nil {
		t.Fatal(err)
	}
	if score.GetFormatScore() < 1850 {
		t.Fatalf("format score=%d want remux+hdr", score.GetFormatScore())
	}
}

func TestOfficialWebTierAndX265(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()
	if _, err := m.SyncTrashGuides(ctx, &formatsv1.SyncTrashGuidesRequest{
		ImportProfiles: true,
		Services:       []string{"radarr"},
	}); err != nil {
		t.Fatal(err)
	}
	web, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title: "Dune.2024.1080p.WEB-DL.FLUX.mkv",
	})
	if err != nil {
		t.Fatal(err)
	}
	if web.GetFormatScore() < 1700 {
		t.Fatalf("FLUX WEB-DL score=%d matches=%v", web.GetFormatScore(), web.GetFormatMatches())
	}
	var webTier bool
	for _, hit := range web.GetFormatMatches() {
		if hit.GetFormatName() == "WEB Tier 01" {
			webTier = true
		}
	}
	if !webTier {
		t.Fatalf("expected WEB Tier 01 match, got %v", web.GetFormatMatches())
	}
	hevc, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title: "Movie.2024.1080p.BluRay.x265.mkv",
	})
	if err != nil {
		t.Fatal(err)
	}
	var x265 bool
	for _, hit := range hevc.GetFormatMatches() {
		if hit.GetFormatName() == "x265" {
			x265 = true
		}
	}
	if !x265 {
		t.Fatalf("expected x265 match, got %v", hevc.GetFormatMatches())
	}
	profiles, err := m.ListProfiles(ctx, &formatsv1.ListProfilesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var uhd bool
	for _, p := range profiles.GetProfiles() {
		if p.GetName() == "UHD Bluray + WEB" && len(p.GetFormatScores()) >= 6 {
			uhd = true
		}
	}
	if !uhd {
		t.Fatal("expected official UHD Bluray + WEB profile scores")
	}
}

func TestLanguageNotEnglishAndFreeleech(t *testing.T) {
	raw, err := os.ReadFile("guides-fixture/docs/json/radarr/cf/official-language-not-english.json")
	if err != nil {
		t.Fatal(err)
	}
	notEN, err := parseTrashFormat(raw, "default")
	if err != nil {
		t.Fatal(err)
	}
	if notEN.GetDefaultScore() != -10000 {
		t.Fatalf("not-english score=%d", notEN.GetDefaultScore())
	}
	ok, _ := matchFormat(notEN, "Movie.2024.1080p.GERMAN.mkv", 0, 0)
	if !ok {
		t.Fatal("German title must match Language: Not English")
	}
	ok, _ = matchFormat(notEN, "Movie.2024.1080p.EN.mkv", 0, 0)
	if ok {
		t.Fatal("English-token title must not match Language: Not English")
	}
	ok, _ = matchFormat(notEN, "Dune.2024.1080p.WEB-DL.FLUX.mkv", 0, 0)
	if ok {
		t.Fatal("English release without a language token must not match Language: Not English")
	}

	flRaw, err := os.ReadFile("guides-fixture/docs/json/radarr/cf/official-freeleech.json")
	if err != nil {
		t.Fatal(err)
	}
	fl, err := parseTrashFormat(flRaw, "default")
	if err != nil {
		t.Fatal(err)
	}
	ok, _ = matchFormat(fl, "Movie.2024.1080p.FREELEECH.mkv", 0, 0)
	if !ok {
		t.Fatal("expected FreeLeech title token to match")
	}
	ok, _ = matchFormat(fl, "Movie.2024.1080p.WEB-DL.mkv", 0, 0)
	if ok {
		t.Fatal("plain WEB-DL must not match FreeLeech")
	}

	m := newTestModule(t)
	ctx := context.Background()
	if _, err := m.SyncTrashGuides(ctx, &formatsv1.SyncTrashGuidesRequest{
		Services: []string{"radarr"},
	}); err != nil {
		t.Fatal(err)
	}
	german, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title: "Movie.2024.1080p.GERMAN.mkv",
	})
	if err != nil {
		t.Fatal(err)
	}
	var hitNotEN bool
	for _, hit := range german.GetFormatMatches() {
		if hit.GetFormatName() == "Language: Not English" {
			hitNotEN = true
		}
	}
	if !hitNotEN {
		t.Fatalf("expected Language: Not English on German title, score=%d matches=%v", german.GetFormatScore(), german.GetFormatMatches())
	}
	english, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title: "Movie.2024.1080p.EN.mkv",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range english.GetFormatMatches() {
		if hit.GetFormatName() == "Language: Not English" {
			t.Fatalf("English title matched Not English: %v", english.GetFormatMatches())
		}
	}
}
