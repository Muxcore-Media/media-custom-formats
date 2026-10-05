package internal

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Muxcore-Media/core/sdk/go/module/pathguard"
)

const (
	officialTrashGuidesSentinel = "official"
	officialTrashGuidesZipURL   = "https://github.com/TRaSH-Guides/Guides/archive/refs/heads/master.zip"
	officialTrashZipMaxBytes    = 80 << 20
	officialTrashMaxFiles       = 20000
)

func isOfficialGuidesSentinel(path string) bool {
	switch strings.ToLower(strings.TrimSpace(path)) {
	case officialTrashGuidesSentinel, "trash-official", "official-zip", "official-refresh":
		return true
	default:
		return false
	}
}

func officialRefreshRequested(path string) bool {
	return strings.EqualFold(strings.TrimSpace(path), "official-refresh")
}

func (m *Module) officialTrashZipURL() string {
	if u := strings.TrimSpace(m.officialTrashURL); u != "" {
		return u
	}
	return officialTrashGuidesZipURL
}

func (m *Module) trashHTTPClient() *http.Client {
	if m.trashHTTP != nil {
		return m.trashHTTP
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func (m *Module) officialCacheDir() string {
	return filepath.Join(filepath.Dir(m.dbPath), "trash-guides-official")
}

func (m *Module) fetchOfficialTrashGuides(ctx context.Context, force bool) (string, error) {
	dest := m.officialCacheDir()
	if force {
		_ = os.RemoveAll(dest)
	} else if root := findGuidesJSONRoot(dest); root != "" {
		return dest, nil
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", fmt.Errorf("create cache: %w", err)
	}
	zipPath := dest + ".zip"
	if err := m.downloadOfficialTrashZip(ctx, zipPath); err != nil {
		return "", err
	}
	if err := extractOfficialTrashZip(zipPath, dest); err != nil {
		return "", err
	}
	_ = os.Remove(zipPath)
	if findGuidesJSONRoot(dest) == "" {
		return "", fmt.Errorf("downloaded archive is not a TRaSH Guides tree")
	}
	return dest, nil
}

func (m *Module) downloadOfficialTrashZip(ctx context.Context, dest string) error {
	url := m.officialTrashZipURL()
	if m.officialTrashURL == "" && !strings.HasPrefix(url, officialTrashGuidesZipURL) {
		return fmt.Errorf("refusing unofficial TRaSH Guides URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("official zip request: %w", err)
	}
	req.Header.Set("User-Agent", "MuxCore-media-custom-formats/0.1.11")
	resp, err := m.trashHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("download official TRaSH Guides: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download official TRaSH Guides: HTTP %d", resp.StatusCode)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create zip: %w", err)
	}
	written, err := io.Copy(f, io.LimitReader(resp.Body, officialTrashZipMaxBytes+1))
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write zip: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if written > officialTrashZipMaxBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("official TRaSH Guides zip exceeds %d bytes", officialTrashZipMaxBytes)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func extractOfficialTrashZip(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()
	if len(r.File) > officialTrashMaxFiles {
		return fmt.Errorf("official TRaSH Guides zip has too many files")
	}
	var uncompressed int64
	for _, f := range r.File {
		uncompressed += int64(f.UncompressedSize64)
		if uncompressed > officialTrashZipMaxBytes {
			return fmt.Errorf("official TRaSH Guides zip expands past %d bytes", officialTrashZipMaxBytes)
		}
		target, err := safeZipPath(dest, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(rc, officialTrashZipMaxBytes+1))
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func safeZipPath(dest, name string) (string, error) {
	norm := strings.ReplaceAll(name, "\\", "/")
	cleaned := filepath.Clean(norm)
	if cleaned == "." || cleaned == "" {
		abs, err := filepath.Abs(dest)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	target, err := pathguard.Join(absDest, cleaned)
	if err != nil {
		return "", fmt.Errorf("illegal zip path %q: %w", name, err)
	}
	return target, nil
}
