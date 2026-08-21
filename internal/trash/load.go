package trash

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadRoot loads metadata.json and discovers custom-format / quality-profile JSON files.
func LoadRoot(root string) (*Metadata, error) {
	raw, err := os.ReadFile(filepath.Join(root, "metadata.json"))
	if err != nil {
		return nil, fmt.Errorf("read metadata.json: %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("parse metadata.json: %w", err)
	}
	if len(meta.JSONPaths) == 0 {
		return nil, fmt.Errorf("metadata.json has no json_paths")
	}
	return &meta, nil
}

// LoadCustomFormats reads all custom format JSON files for the given services.
func LoadCustomFormats(root string, meta *Metadata, services []string) ([]struct {
	Service string
	Format  CustomFormatJSON
	Path    string
}, error) {
	services = normalizeServices(services, meta)
	var out []struct {
		Service string
		Format  CustomFormatJSON
		Path    string
	}
	for _, svc := range services {
		paths, ok := meta.JSONPaths[svc]
		if !ok {
			continue
		}
		for _, rel := range paths.CustomFormats {
			dir := filepath.Join(root, rel)
			entries, err := os.ReadDir(dir)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", dir, err)
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
					continue
				}
				p := filepath.Join(dir, e.Name())
				cf, err := readCustomFormat(p)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", p, err)
				}
				out = append(out, struct {
					Service string
					Format  CustomFormatJSON
					Path    string
				}{Service: svc, Format: cf, Path: p})
			}
		}
	}
	return out, nil
}

// LoadQualityProfiles reads quality profile JSON files for the given services.
func LoadQualityProfiles(root string, meta *Metadata, services []string) ([]struct {
	Service string
	Profile QualityProfileJSON
	Path    string
}, error) {
	services = normalizeServices(services, meta)
	var out []struct {
		Service string
		Profile QualityProfileJSON
		Path    string
	}
	for _, svc := range services {
		paths, ok := meta.JSONPaths[svc]
		if !ok {
			continue
		}
		for _, rel := range paths.QualityProfiles {
			dir := filepath.Join(root, rel)
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("read %s: %w", dir, err)
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
					continue
				}
				p := filepath.Join(dir, e.Name())
				qp, err := readQualityProfile(p)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", p, err)
				}
				out = append(out, struct {
					Service string
					Profile QualityProfileJSON
					Path    string
				}{Service: svc, Profile: qp, Path: p})
			}
		}
	}
	return out, nil
}

func normalizeServices(services []string, meta *Metadata) []string {
	if len(services) == 0 {
		out := make([]string, 0, len(meta.JSONPaths))
		for k := range meta.JSONPaths {
			out = append(out, k)
		}
		// stable-ish order
		prefer := []string{"radarr", "sonarr"}
		ordered := make([]string, 0, len(out))
		seen := map[string]bool{}
		for _, p := range prefer {
			if _, ok := meta.JSONPaths[p]; ok {
				ordered = append(ordered, p)
				seen[p] = true
			}
		}
		for _, s := range out {
			if !seen[s] {
				ordered = append(ordered, s)
			}
		}
		return ordered
	}
	return services
}

func readCustomFormat(path string) (CustomFormatJSON, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return CustomFormatJSON{}, err
	}
	var cf CustomFormatJSON
	if err := json.Unmarshal(raw, &cf); err != nil {
		return CustomFormatJSON{}, err
	}
	return cf, nil
}

func readQualityProfile(path string) (QualityProfileJSON, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return QualityProfileJSON{}, err
	}
	var qp QualityProfileJSON
	if err := json.Unmarshal(raw, &qp); err != nil {
		return QualityProfileJSON{}, err
	}
	return qp, nil
}
