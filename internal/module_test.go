package internal

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

func newTestModule(t *testing.T) *Module {
	t.Helper()
	m := NewModule(Config{
		DBPath:   filepath.Join(t.TempDir(), "formats.db"),
		GRPCAddr: ":0",
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })
	return m
}

func TestSeedDefaults(t *testing.T) {
	m := NewModule(Config{
		DBPath:       filepath.Join(t.TempDir(), "formats.db"),
		GRPCAddr:     "127.0.0.1:0",
		SeedDefaults: true,
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	list, err := m.ListFormats(ctx, &formatsv1.ListFormatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int32{
		"Remux":         100,
		"HDR":           50,
		"x265/HEVC":     25,
		"Proper/Repack": 20,
		"CAM/TS":        -10000,
	}
	if len(list.Formats) != len(want) {
		t.Fatalf("seeded formats = %d want %d", len(list.Formats), len(want))
	}
	for _, f := range list.Formats {
		score, ok := want[f.GetName()]
		if !ok {
			t.Fatalf("unexpected seed %q", f.GetName())
		}
		if f.GetDefaultScore() != score {
			t.Fatalf("%s score = %d want %d", f.GetName(), f.GetDefaultScore(), score)
		}
	}

	var groupName string
	err = m.db.QueryRowContext(ctx, `SELECT name FROM release_profile_groups WHERE id = ?`, "rpg_seed_default").Scan(&groupName)
	if err != nil {
		t.Fatalf("seeded release group: %v", err)
	}
	if groupName != "Default Blocklist" {
		t.Fatalf("release group name = %q", groupName)
	}
}

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version == "" {
		t.Error("module version must not be empty")
	}
	if len(info.Capabilities) == 0 || info.Capabilities[0] != "media.scoring" {
		t.Errorf("expected media.scoring, got %v", info.Capabilities)
	}
}

func TestReleaseProfileCRUDAndReject(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	off := false
	created, err := m.UpsertReleaseProfile(ctx, &formatsv1.UpsertReleaseProfileRequest{
		Name:           "No CAM",
		MustNotContain: []string{"cam", "telesync"},
		Preferred:      []string{"bluray"},
		PreferredScore: 20,
		Enabled:        protoBool(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Profile.GetId() == "" || !created.Profile.GetEnabled() {
		t.Fatalf("created %#v", created.Profile)
	}
	listed, err := m.ListReleaseProfiles(ctx, &formatsv1.ListReleaseProfilesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Profiles) != 1 {
		t.Fatalf("listed %d", len(listed.Profiles))
	}
	rejected, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{Title: "Movie.2024.CAM.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.GetTotalScore() != -100000 {
		t.Fatalf("expected reject score, got %d", rejected.GetTotalScore())
	}
	bonus, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{Title: "Movie.2024.Bluray.1080p.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if bonus.GetTotalScore() < 20 {
		t.Fatalf("expected preferred bonus, got %d", bonus.GetTotalScore())
	}
	if _, err := m.UpsertReleaseProfile(ctx, &formatsv1.UpsertReleaseProfileRequest{
		Id: created.Profile.GetId(), Name: "No CAM", Enabled: &off,
	}); err != nil {
		t.Fatal(err)
	}
	allowed, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{Title: "Movie.2024.CAM.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if allowed.GetTotalScore() == -100000 {
		t.Fatal("paused restriction still rejected")
	}
	if _, err := m.DeleteReleaseProfile(ctx, &formatsv1.DeleteReleaseProfileRequest{Id: created.Profile.GetId()}); err != nil {
		t.Fatal(err)
	}
	empty, err := m.ListReleaseProfiles(ctx, &formatsv1.ListReleaseProfilesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Profiles) != 0 {
		t.Fatalf("expected empty after delete, got %d", len(empty.Profiles))
	}
}

func protoBool(v bool) *bool { return &v }

func TestCreateAndListFormat(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	created, err := m.CreateFormat(ctx, &formatsv1.CreateFormatRequest{
		Name: "HDR10",
		Rules: []*formatsv1.FormatRule{
			{Field: "title", Op: "matches", Value: "(?i)hdr"},
		},
		DefaultScore: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Format.Name != "HDR10" {
		t.Errorf("expected HDR10, got %s", created.Format.Name)
	}

	list, err := m.ListFormats(ctx, &formatsv1.ListFormatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Formats) != 1 {
		t.Errorf("expected 1 format, got %d", len(list.Formats))
	}
}

func TestUpdateFormat(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	created, _ := m.CreateFormat(ctx, &formatsv1.CreateFormatRequest{
		Name: "HDR10", DefaultScore: 50,
	})

	updated, err := m.UpdateFormat(ctx, &formatsv1.UpdateFormatRequest{
		Id: created.Format.Id, Name: "HDR10+", DefaultScore: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Format.Name != "HDR10+" {
		t.Errorf("expected HDR10+, got %s", updated.Format.Name)
	}
	if updated.Format.DefaultScore != 100 {
		t.Errorf("expected score 100, got %d", updated.Format.DefaultScore)
	}
}

func TestDeleteFormat(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	created, _ := m.CreateFormat(ctx, &formatsv1.CreateFormatRequest{Name: "Test", DefaultScore: 10})
	_, err := m.DeleteFormat(ctx, &formatsv1.DeleteFormatRequest{Id: created.Format.Id})
	if err != nil {
		t.Fatal(err)
	}

	list, _ := m.ListFormats(ctx, &formatsv1.ListFormatsRequest{})
	if len(list.Formats) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(list.Formats))
	}
}

func TestCreateAndListProfile(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	created, err := m.CreateProfile(ctx, &formatsv1.CreateProfileRequest{
		Name:           "HD-1080p",
		MinScore:       100,
		CutoffScore:    200,
		UpgradeAllowed: true,
		FormatScores:   map[string]int32{"cf1": 50},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Profile.Name != "HD-1080p" {
		t.Errorf("expected HD-1080p, got %s", created.Profile.Name)
	}
	if created.Profile.MinScore != 100 {
		t.Errorf("expected min 100, got %d", created.Profile.MinScore)
	}
	if !created.Profile.UpgradeAllowed {
		t.Error("expected upgrade allowed")
	}
	if created.Profile.FormatScores["cf1"] != 50 {
		t.Errorf("expected format score 50, got %d", created.Profile.FormatScores["cf1"])
	}

	list, err := m.ListProfiles(ctx, &formatsv1.ListProfilesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(list.Profiles))
	}
}

func TestUpdateProfile(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	created, _ := m.CreateProfile(ctx, &formatsv1.CreateProfileRequest{Name: "Test", MinScore: 100, CutoffScore: 200})

	updated, err := m.UpdateProfile(ctx, &formatsv1.UpdateProfileRequest{
		Id: created.Profile.Id, Name: "Updated", MinScore: 150, CutoffScore: 250,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Profile.Name != "Updated" || updated.Profile.MinScore != 150 {
		t.Errorf("update not applied: %+v", updated.Profile)
	}
}

func TestDeleteProfile(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	created, _ := m.CreateProfile(ctx, &formatsv1.CreateProfileRequest{Name: "Test", MinScore: 0, CutoffScore: 100})
	if _, err := m.DeleteProfile(ctx, &formatsv1.DeleteProfileRequest{Id: created.Profile.Id}); err != nil {
		t.Fatal(err)
	}

	list, _ := m.ListProfiles(ctx, &formatsv1.ListProfilesRequest{})
	if len(list.Profiles) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(list.Profiles))
	}
}

func TestParseQuality(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	tests := []struct {
		title      string
		resolution string
		source     string
		codec      string
		hdr        bool
	}{
		{"Movie.2020.2160p.Remux.HEVC.HDR.mkv", "2160p", "Remux", "hevc", true},
		{"Show.S01E01.1080p.BluRay.x264.mkv", "1080p", "BluRay", "h264", false},
		{"Film.2020.720p.WEB-DL.AVC.mkv", "720p", "WEB-DL", "h264", false},
		{"Test.2020.1080p.WEB-DL.HDR.DV.mkv", "1080p", "WEB-DL", "", true},
	}
	for _, tt := range tests {
		resp, err := m.ParseQuality(ctx, &formatsv1.ParseQualityRequest{Title: tt.title})
		if err != nil {
			t.Fatal(err)
		}
		q := resp.Quality
		if q.Resolution != tt.resolution {
			t.Errorf("%s: resolution=%s want %s", tt.title, q.Resolution, tt.resolution)
		}
		if q.Source != tt.source {
			t.Errorf("%s: source=%s want %s", tt.title, q.Source, tt.source)
		}
	}
}

func TestScoreReleaseNoFormats(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	resp, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title: "Movie.2020.1080p.BluRay.x264.mkv",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.TotalScore <= 0 {
		t.Errorf("expected positive score, got %d", resp.TotalScore)
	}
	if resp.Quality.Resolution != "1080p" {
		t.Errorf("expected 1080p, got %s", resp.Quality.Resolution)
	}
	if resp.QualityScore <= 0 {
		t.Errorf("expected quality score > 0, got %d", resp.QualityScore)
	}
}

func TestScoreReleaseWithFormats(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	if _, err := m.CreateFormat(ctx, &formatsv1.CreateFormatRequest{
		Name: "HDR",
		Rules: []*formatsv1.FormatRule{
			{Field: "title", Op: "matches", Value: "(?i)hdr|dolby.?vision"},
		},
		DefaultScore: 50,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateFormat(ctx, &formatsv1.CreateFormatRequest{
		Name: "x265",
		Rules: []*formatsv1.FormatRule{
			{Field: "title", Op: "matches", Value: "(?i)x265|hevc"},
		},
		DefaultScore: 25,
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title: "Movie.2020.2160p.Remux.HEVC.HDR.mkv",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.FormatMatches) != 2 {
		t.Errorf("expected 2 format matches (HDR + x265), got %d", len(resp.FormatMatches))
	}
	if resp.FormatScore <= 0 {
		t.Errorf("expected format score > 0, got %d", resp.FormatScore)
	}
}

func TestScoreReleaseWithProfile(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	cf, _ := m.CreateFormat(ctx, &formatsv1.CreateFormatRequest{
		Name: "HDR", DefaultScore: 50,
	})
	p, _ := m.CreateProfile(ctx, &formatsv1.CreateProfileRequest{
		Name: "My Profile", MinScore: 100, CutoffScore: 200,
		FormatScores: map[string]int32{cf.Format.Id: 100},
	})

	resp, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title:     "Movie.2020.2160p.Remux.HEVC.HDR.mkv",
		ProfileId: p.Profile.Id,
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.FormatScore != 100 {
		t.Errorf("expected format score 100 (from profile override), got %d", resp.FormatScore)
	}
}

func TestMatchRule(t *testing.T) {
	tests := []struct {
		field, op, value string
		title            string
		size             int64
		seeders          int32
		negate           bool
		want             bool
	}{
		{"title", "contains", "hdr", "Movie HDR", 0, 0, false, true},
		{"title", "contains", "hdr", "Movie SDR", 0, 0, false, false},
		{"title", "matches", "(?i)^.+2160p.+$", "Movie.2160p.Remux.mkv", 0, 0, false, true},
		{"size", "gt", "10000000000", "Movie", 15000000000, 0, false, true},
		{"size", "lt", "10000000000", "Movie", 5000000000, 0, false, true},
		{"seeders", "gte", "10", "Movie", 0, 20, false, true},
		{"seeders", "lt", "5", "Movie", 0, 3, false, true},
		{"title", "contains", "bad", "Good Movie", 0, 0, true, true},
	}
	for _, tt := range tests {
		rule := &formatsv1.FormatRule{
			Field: tt.field, Op: tt.op, Value: tt.value, Negate: tt.negate,
		}
		got := matchRule(rule, tt.title, tt.size, tt.seeders)
		if got != tt.want {
			t.Errorf("matchRule(%q,%q,%q) on %q = %v, want %v", tt.field, tt.op, tt.value, tt.title, got, tt.want)
		}
	}
}

func TestLifecycle(t *testing.T) {
	m := NewModule(Config{
		DBPath:   filepath.Join(t.TempDir(), "lifecycle.db"),
		GRPCAddr: ":0",
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestHealth(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()
	if err := m.Health(ctx); err != nil {
		t.Fatal("expected health to pass")
	}
}
