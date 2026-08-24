package trash

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultArchiveURL = "https://github.com/TRaSH-Guides/Guides/archive/refs/heads/master.tar.gz"

// ResolveRoot returns a local Guides root.
// If path is set and valid, it is used. Otherwise archiveURL is downloaded into cacheDir.
func ResolveRoot(path, archiveURL, cacheDir string) (string, error) {
	path = strings.TrimSpace(path)
	if path != "" {
		if _, err := os.Stat(filepath.Join(path, "metadata.json")); err != nil {
			return "", fmt.Errorf("FORMATS_TRASH_GUIDES_PATH %q missing metadata.json: %w", path, err)
		}
		return path, nil
	}
	if archiveURL == "" {
		archiveURL = DefaultArchiveURL
	}
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "muxcore-trash-guides")
	}
	dest := filepath.Join(cacheDir, "Guides")
	if _, err := os.Stat(filepath.Join(dest, "metadata.json")); err == nil {
		return dest, nil
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", err
	}
	if err := downloadAndExtract(archiveURL, cacheDir); err != nil {
		return "", err
	}
	// GitHub archive extracts to Guides-master / Guides-<sha>
	matches, _ := filepath.Glob(filepath.Join(cacheDir, "Guides*"))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(m, "metadata.json")); err == nil {
			// normalize to cacheDir/Guides via rename if needed
			if m != dest {
				_ = os.RemoveAll(dest)
				if err := os.Rename(m, dest); err != nil {
					return m, nil // usable even if rename fails
				}
			}
			return dest, nil
		}
	}
	return "", fmt.Errorf("downloaded archive did not contain metadata.json under %s", cacheDir)
}

func downloadAndExtract(url, destDir string) error {
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download trash guides: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download trash guides: HTTP %s", resp.Status)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		// Only extract metadata + docs/json (custom formats / profiles / groups)
		name := hdr.Name
		if i := strings.Index(name, "/"); i >= 0 {
			// strip leading Guides-master/
			rest := name[i+1:]
			if rest != "metadata.json" && !strings.HasPrefix(rest, "docs/json/") {
				continue
			}
		} else {
			continue
		}
		target := filepath.Join(destDir, name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(destDir) {
			return fmt.Errorf("tar path escapes dest: %s", name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			_ = f.Close()
		}
	}
	return nil
}
