package trash_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Muxcore-Media/media-custom-formats/internal/trash"
)

func writeGuidesArchive(t *testing.T, marker string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	meta := []byte(`{"json_paths":{"radarr":{"custom_formats":["docs/json/radarr/cf"]}}}`)
	if err := tw.WriteHeader(&tar.Header{
		Name: "Guides-fixture/metadata.json", Size: int64(len(meta)), Mode: 0o644, ModTime: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(meta); err != nil {
		t.Fatal(err)
	}
	doc := []byte(`{"trash_id":"cf-fixture","name":"` + marker + `","trash_scores":{"default":1},"specifications":[]}`)
	if err := tw.WriteHeader(&tar.Header{
		Name: "Guides-fixture/docs/json/radarr/cf/fixture.json", Size: int64(len(doc)), Mode: 0o644, ModTime: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(doc); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestResolveRootRefreshesStaleCache(t *testing.T) {
	cacheDir := t.TempDir()
	dest := filepath.Join(cacheDir, "Guides")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(dest, "metadata.json")
	if err := os.WriteFile(metaPath, []byte(`{"json_paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(metaPath, old, old); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(writeGuidesArchive(t, "fresh"))
	}))
	t.Cleanup(srv.Close)

	root, err := trash.ResolveRoot("", srv.URL, cacheDir, trash.ResolveOptions{MaxAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if root != dest {
		t.Fatalf("root=%q want %q", root, dest)
	}
	info, err := os.Stat(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(info.ModTime()) > time.Minute {
		t.Fatalf("metadata not refreshed: modtime=%v", info.ModTime())
	}
}

func TestResolveRootForceRefresh(t *testing.T) {
	cacheDir := t.TempDir()

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write(writeGuidesArchive(t, "forced"))
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(cacheDir, "Guides")
	root1, err := trash.ResolveRoot("", srv.URL, cacheDir, trash.ResolveOptions{Force: false, MaxAge: trash.DefaultCacheMaxAge})
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("initial download hits=%d want 1", hits)
	}
	root2, err := trash.ResolveRoot("", srv.URL, cacheDir, trash.ResolveOptions{Force: true, MaxAge: trash.DefaultCacheMaxAge})
	if err != nil {
		t.Fatal(err)
	}
	if root1 != dest || root2 != dest {
		t.Fatalf("roots=%q %q want %q", root1, root2, dest)
	}
	if hits != 2 {
		t.Fatalf("download hits=%d want 2 (initial + forced refresh)", hits)
	}
	if _, err := os.Stat(filepath.Join(dest, "metadata.json")); err != nil {
		t.Fatal(err)
	}
}
