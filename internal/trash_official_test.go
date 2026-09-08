package internal

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

const officialZipProbeJSON = `{
  "trash_id": "official-zip-probe",
  "trash_scores": { "default": 42 },
  "name": "Official Zip Probe",
  "specifications": [
    {
      "name": "FLUX",
      "implementation": "ReleaseGroupSpecification",
      "negate": false,
      "required": true,
      "fields": { "value": "FLUX" }
    }
  ]
}`

func writeOfficialFixtureZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("Guides-master/docs/json/radarr/cf/official-zip-probe.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, officialZipProbeJSON); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSyncTrashGuidesOfficialZip(t *testing.T) {
	zipBytes := writeOfficialFixtureZip(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipBytes)
	}))
	t.Cleanup(srv.Close)

	m := newTestModule(t)
	m.officialTrashURL = srv.URL
	m.trashHTTP = srv.Client()

	resp, err := m.SyncTrashGuides(context.Background(), &formatsv1.SyncTrashGuidesRequest{
		GuidesPath: officialTrashGuidesSentinel,
		Services:   []string{"radarr"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetFormatsUpserted() < 1 {
		t.Fatalf("upserted=%d skipped=%d path=%s warnings=%v", resp.GetFormatsUpserted(), resp.GetFormatsSkipped(), resp.GetGuidesPath(), resp.GetWarnings())
	}
	list, err := m.ListFormats(context.Background(), &formatsv1.ListFormatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range list.GetFormats() {
		if f.GetName() == "Official Zip Probe" && f.GetDefaultScore() == 42 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Official Zip Probe, got %#v", list.GetFormats())
	}

	// Second sync uses the cached extract, not the HTTP server.
	srv.Close()
	again, err := m.SyncTrashGuides(context.Background(), &formatsv1.SyncTrashGuidesRequest{
		GuidesPath: "official",
		Services:   []string{"radarr"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.GetFormatsUpserted() < 1 {
		t.Fatalf("cached official sync upserted=%d", again.GetFormatsUpserted())
	}
}

func TestSyncTrashGuidesOfficialDoesNotUseClientURL(t *testing.T) {
	m := newTestModule(t)
	m.officialTrashURL = ""
	// A filesystem path that is not the sentinel stays local (no download).
	resp, err := m.SyncTrashGuides(context.Background(), &formatsv1.SyncTrashGuidesRequest{
		GuidesPath: filepath.Join(t.TempDir(), "missing"),
		Services:   []string{"radarr"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetGuidesPath() != "embedded:guides-fixture" {
		t.Fatalf("missing path should fall back to embedded, got %s", resp.GetGuidesPath())
	}
}

func TestSafeZipPathRejectsTraversal(t *testing.T) {
	dest := t.TempDir()
	if _, err := safeZipPath(dest, "../etc/passwd"); err == nil {
		t.Fatal("expected traversal reject")
	}
	if _, err := safeZipPath(dest, "Guides-master/../../secret"); err == nil {
		t.Fatal("expected nested traversal reject")
	}
	got, err := safeZipPath(dest, "Guides-master/docs/json/radarr/cf/a.json")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "a.json" {
		t.Fatalf("got %s", got)
	}
}

func TestExtractOfficialTrashZipRejectsSlip(t *testing.T) {
	dest := t.TempDir()
	zipPath := filepath.Join(dest, "bad.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../evil.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "{}")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extractOfficialTrashZip(zipPath, filepath.Join(dest, "out")); err == nil {
		t.Fatal("expected zip slip reject")
	}
}
