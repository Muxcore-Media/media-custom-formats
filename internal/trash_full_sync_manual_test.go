package internal

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

// TestFullTrashGuidesTree syncs the complete official Guides tree when TRASH_GUIDES_ROOT is set.
// Example: TRASH_GUIDES_ROOT=/path/to/TRaSH-Guides/Guides go test -run TestFullTrashGuidesTree ./internal
func TestFullTrashGuidesTree(t *testing.T) {
	root := os.Getenv("TRASH_GUIDES_ROOT")
	if root == "" {
		t.Skip("set TRASH_GUIDES_ROOT to a TRaSH Guides checkout to run full-tree sync")
	}
	if _, err := os.Stat(filepath.Join(root, "metadata.json")); err != nil {
		t.Fatal(err)
	}
	m := NewModule(Config{
		DBPath:       filepath.Join(t.TempDir(), "full.db"),
		GRPCAddr:     ":0",
		SeedDefaults: false,
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })
	resp, err := m.SyncTrashGuides(ctx, &formatsv1.SyncTrashGuidesRequest{
		Path:           root,
		ImportProfiles: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("upserted=%d skipped=%d profiles=%d warnings=%d", resp.FormatsUpserted, resp.FormatsSkipped, resp.ProfilesUpserted, len(resp.Warnings))
	if resp.FormatsUpserted < 400 {
		n := len(resp.Warnings)
		if n > 10 {
			n = 10
		}
		t.Fatalf("expected most of ~476 CFs, got upserted=%d skipped=%d first warnings=%v", resp.FormatsUpserted, resp.FormatsSkipped, resp.Warnings[:n])
	}
	score, err := m.ScoreRelease(ctx, &formatsv1.ScoreReleaseRequest{
		Title:    "Movie.2024.2160p.BluRay.REMUX.HDR10-FraMeSToR",
		Category: "movie",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(score.GetFormatMatches()) == 0 {
		t.Fatalf("expected trash-driven matches, got %+v", score)
	}
	t.Logf("score total=%d format=%d matches=%d", score.TotalScore, score.FormatScore, len(score.FormatMatches))
}
