package internal

import (
	"context"
	"path/filepath"
	"testing"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

func newSeededTestModule(t *testing.T) *Module {
	t.Helper()
	m := NewModule(Config{
		DBPath:       filepath.Join(t.TempDir(), "formats.db"),
		GRPCAddr:     ":0",
		SeedDefaults: true,
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { m.Stop(ctx) })
	return m
}

// TestScoreReleaseGoldenSeeds locks exact scores for seeded formats + release groups.
func TestScoreReleaseGoldenSeeds(t *testing.T) {
	m := newSeededTestModule(t)
	ctx := context.Background()

	cases := []struct {
		name         string
		title        string
		wantTotal    int32
		wantQuality  int32
		wantFmtMin   int32 // format score lower bound (CAM / rejection paths may vary)
		wantRejected bool
		wantRes      string
		wantSource   string
		wantFmtNames []string
	}{
		{
			name:         "1080p_bluray",
			title:        "Fight.Club.1999.1080p.BluRay.x264",
			wantQuality:  130, // 100 + 30
			wantTotal:    145, // +15 preferred bluray
			wantRes:      "1080p",
			wantSource:   "BluRay",
			wantFmtNames: nil,
		},
		{
			name:         "2160p_remux_hevc_hdr",
			title:        "Movie.2020.2160p.Remux.HEVC.HDR.mkv",
			wantQuality:  170, // 120 + 40 + 10
			wantTotal:    360, // + Remux100 + HDR50 + x265/25 + preferred remux15
			wantRes:      "2160p",
			wantSource:   "Remux",
			wantFmtNames: []string{"Remux", "HDR", "x265/HEVC"},
		},
		{
			name:        "720p_webdl",
			title:       "Show.S01E01.720p.WEB-DL.x264",
			wantQuality: 100, // 80 + 20
			wantTotal:   115, // +15 preferred web-dl
			wantRes:     "720p",
			wantSource:  "WEB-DL",
		},
		{
			name:         "cam_rejected",
			title:        "Movie.2020.CAM.x264",
			wantRejected: true,
			wantTotal:    -100000,
			wantRes:      "SD",
			wantSource:   "CAM",
		},
		{
			name:         "proper_repack_bonus",
			title:        "Movie.2021.1080p.BluRay.Proper.x264",
			wantQuality:  130,
			wantTotal:    165, // + Proper20 + preferred bluray15
			wantRes:      "1080p",
			wantSource:   "BluRay",
			wantFmtNames: []string{"Proper/Repack"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{Title: tc.title})
			if err != nil {
				t.Fatal(err)
			}
			if resp.GetQuality().GetResolution() != tc.wantRes {
				t.Errorf("resolution: got %q want %q", resp.GetQuality().GetResolution(), tc.wantRes)
			}
			if resp.GetQuality().GetSource() != tc.wantSource {
				t.Errorf("source: got %q want %q", resp.GetQuality().GetSource(), tc.wantSource)
			}
			if !tc.wantRejected && resp.GetQualityScore() != tc.wantQuality {
				t.Errorf("quality_score: got %d want %d", resp.GetQualityScore(), tc.wantQuality)
			}
			if resp.GetTotalScore() != tc.wantTotal {
				t.Errorf("total_score: got %d want %d (quality=%d format=%d matches=%v)",
					resp.GetTotalScore(), tc.wantTotal, resp.GetQualityScore(), resp.GetFormatScore(),
					formatMatchNames(resp))
			}
			for _, name := range tc.wantFmtNames {
				if !hasFormatMatch(resp, name) {
					t.Errorf("missing format match %q in %v", name, formatMatchNames(resp))
				}
			}
		})
	}
}

func formatMatchNames(resp *formatsv1.ScoreReleaseResponse) []string {
	out := make([]string, 0, len(resp.GetFormatMatches()))
	for _, m := range resp.GetFormatMatches() {
		out = append(out, m.GetFormatName())
	}
	return out
}

func hasFormatMatch(resp *formatsv1.ScoreReleaseResponse, name string) bool {
	for _, m := range resp.GetFormatMatches() {
		if m.GetFormatName() == name {
			return true
		}
	}
	return false
}
