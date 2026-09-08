package internal

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

//go:embed guides-fixture
var bundledGuides embed.FS

func trashFormatID(trashID string) string {
	id := strings.TrimSpace(trashID)
	if id == "" {
		return ""
	}
	return "cf_trash_" + id
}

func trashProfileID(trashID string) string {
	id := strings.TrimSpace(trashID)
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, "qp_") {
		return id
	}
	return "qp_trash_" + id
}

type trashCFFile struct {
	TrashID        string           `json:"trash_id"`
	Name           string           `json:"name"`
	TrashScores    map[string]int32 `json:"trash_scores"`
	Specifications []trashSpec      `json:"specifications"`
}

type trashSpec struct {
	Name           string          `json:"name"`
	Implementation string          `json:"implementation"`
	Negate         bool            `json:"negate"`
	Required       bool            `json:"required"`
	Fields         json.RawMessage `json:"fields"`
}

type trashProfileFile struct {
	TrashID           string          `json:"trash_id"`
	Name              string          `json:"name"`
	MinFormatScore    int32           `json:"minFormatScore"`
	CutoffFormatScore int32           `json:"cutoffFormatScore"`
	MinScore          int32           `json:"min_score"`
	CutoffScore       int32           `json:"cutoff_score"`
	UpgradeAllowed    *bool           `json:"upgradeAllowed"`
	UpgradeAllowedAlt *bool           `json:"upgrade_allowed"`
	FormatItems       json.RawMessage `json:"formatItems"`
	FormatItemsAlt    json.RawMessage `json:"format_items"`
}

type trashFormatItem struct {
	TrashID string `json:"trash_id"`
	Name    string `json:"name"`
	Score   int32  `json:"score"`
}

func specValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if v, ok := obj["value"]; ok {
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, row := range arr {
			name := strings.ToLower(fmt.Sprint(row["name"]))
			if name == "value" || name == "" {
				if v, ok := row["value"]; ok {
					return strings.TrimSpace(fmt.Sprint(v))
				}
			}
		}
	}
	return ""
}

func trashScore(scores map[string]int32, scoreSet string) int32 {
	if scores == nil {
		return 0
	}
	if scoreSet != "" {
		if v, ok := scores[scoreSet]; ok {
			return v
		}
	}
	if v, ok := scores["default"]; ok {
		return v
	}
	return 0
}

func groupToTitleRegex(value string) string {
	inner := strings.TrimSpace(value)
	inner = strings.TrimPrefix(inner, "^")
	inner = strings.TrimSuffix(inner, "$")
	inner = strings.TrimSpace(inner)
	if strings.HasPrefix(inner, "(") && strings.HasSuffix(inner, ")") {
		inner = inner[1 : len(inner)-1]
	}
	if inner == "" {
		return ""
	}
	return `(?i)(?:^|[.\-_ \[\]()])(?:` + inner + `)(?:$|[.\-_ \[\]()])`
}

func sourceToTitleRegex(value string) string {
	switch strings.TrimSpace(value) {
	case "1":
		return `(?i)\b(cam|hdcam|camrip)\b`
	case "2":
		return `(?i)\b(ts|telesync|hdts)\b`
	case "3":
		return `(?i)\b(tc|telecine)\b`
	case "4":
		return `(?i)\b(workprint|wp)\b`
	case "5":
		return `(?i)\b(dvdrip|dvd[ .\-]?r)\b`
	case "6":
		return `(?i)\b(hdtv|pdtv|dsr)\b`
	case "7":
		return `(?i)\bweb[ .\-]?dl\b`
	case "8":
		return `(?i)\bweb[ .\-]?rip\b`
	case "9":
		return `(?i)\b(blu[ .\-]?ray|bluray|bdrip|brrip)\b`
	default:
		return ""
	}
}

func resolutionToTitleRegex(value string) string {
	switch strings.TrimSpace(value) {
	case "2160", "2160p":
		return `(?i)\b(2160p|3840x2160|4k)\b`
	case "1080", "1080p":
		return `(?i)\b(1080p|1920x1080)\b`
	case "720", "720p":
		return `(?i)\b(720p|1280x720)\b`
	case "576", "576p":
		return `(?i)\b576p\b`
	case "480", "480p":
		return `(?i)\b480p\b`
	default:
		return ""
	}
}

func modifierToTitleRegex(value string) string {
	switch strings.TrimSpace(value) {
	case "4":
		return `(?i)\b(br[ .\-]?disk|complete[ .\-]?bluray|\.iso\b)`
	case "5":
		return `(?i)\bremux\b`
	case "2":
		return `(?i)\b(screener|scr)\b`
	default:
		return ""
	}
}

func languageToTitleRegex(value string) string {
	// Radarr Language enum IDs — title-token approximation for household scoring.
	var tokens string
	switch strings.TrimSpace(value) {
	case "1":
		tokens = "en|eng|english"
	case "2":
		tokens = "french|francais|fra|vf|vostfr|truefrench"
	case "3":
		tokens = "spanish|espanol|spa|castellano"
	case "4":
		tokens = "german|deutsch|ger|deu"
	case "5":
		tokens = "italian|italiano|ita"
	case "6":
		tokens = "danish|dan"
	case "7":
		tokens = "dutch|nld|nl"
	case "8":
		tokens = "japanese|nihongo|jap|jpn"
	case "9":
		tokens = "icelandic|isl"
	case "10":
		tokens = "chinese|mandarin|cantonese|chi|zho"
	case "11":
		tokens = "russian|rus"
	case "12":
		tokens = "polish|pol"
	case "13":
		tokens = "vietnamese|vie"
	case "14":
		tokens = "swedish|swe"
	case "15":
		tokens = "norwegian|nor"
	case "16":
		tokens = "finnish|fin"
	case "17":
		tokens = "turkish|tur"
	case "18":
		tokens = "portuguese|por"
	case "19":
		tokens = "flemish"
	case "20":
		tokens = "greek|ell"
	case "21":
		tokens = "korean|kor"
	case "22":
		tokens = "hungarian|hun"
	case "23":
		tokens = "hebrew|heb"
	case "24":
		tokens = "lithuanian|lit"
	case "25":
		tokens = "czech|cze|ces"
	case "26":
		tokens = "arabic|ara"
	case "27":
		tokens = "hindi|hin"
	case "28":
		tokens = "bulgarian|bul"
	case "29":
		tokens = "malayalam|mal"
	case "30":
		tokens = "ukrainian|ukr"
	case "31":
		tokens = "slovak|slk"
	case "32":
		tokens = "thai|tha"
	case "33":
		tokens = "brazilian|ptbr|pt-br"
	case "34":
		tokens = "latino|espanol[ .\\-]?latino"
	default:
		return ""
	}
	return `(?i)(?:^|[.\-_ \[\]()])(?:` + tokens + `)(?:$|[.\-_ \[\]()])`
}

func indexerFlagToTitleRegex(value string) string {
	// Radarr IndexerFlags — only tokens that sometimes appear in release titles.
	switch strings.TrimSpace(value) {
	case "1":
		return `(?i)(?:^|[.\-_ \[\]()])(?:freeleech|free[ .\-]?leech|\bfl\b)(?:$|[.\-_ \[\]()])`
	case "2":
		return `(?i)(?:^|[.\-_ \[\]()])(?:halfleech|half[ .\-]?leech)(?:$|[.\-_ \[\]()])`
	case "4":
		return `(?i)(?:^|[.\-_ \[\]()])(?:double[ .\-]?upload)(?:$|[.\-_ \[\]()])`
	case "8":
		return `(?i)(?:^|[.\-_ \[\]()])internal(?:$|[.\-_ \[\]()])`
	case "16":
		return `(?i)(?:^|[.\-_ \[\]()])scene(?:$|[.\-_ \[\]()])`
	case "32":
		return `(?i)(?:^|[.\-_ \[\]()])(?:freeleech75|fl75)(?:$|[.\-_ \[\]()])`
	case "64":
		return `(?i)(?:^|[.\-_ \[\]()])(?:freeleech25|fl25)(?:$|[.\-_ \[\]()])`
	default:
		return ""
	}
}

func nonEnglishLanguageToTitleRegex() string {
	var parts []string
	for _, id := range []string{
		"2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14",
		"15", "16", "17", "18", "19", "20", "21", "22", "23", "24", "25", "26",
		"27", "28", "29", "30", "31", "32", "33", "34",
	} {
		rx := languageToTitleRegex(id)
		if rx == "" {
			continue
		}
		parts = append(parts, "(?:"+rx+")")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "|")
}

func specToTitleMatch(spec trashSpec) (rx string, negate bool) {
	impl := strings.ToLower(spec.Implementation)
	value := specValue(spec.Fields)
	negate = spec.Negate
	switch {
	case strings.Contains(impl, "releasetitle") || strings.Contains(impl, "release_title"):
		return value, negate
	case strings.Contains(impl, "releasegroup") || strings.Contains(impl, "release_group"):
		return groupToTitleRegex(value), negate
	case strings.Contains(impl, "source"):
		return sourceToTitleRegex(value), negate
	case strings.Contains(impl, "resolution"):
		return resolutionToTitleRegex(value), negate
	case strings.Contains(impl, "qualitymodifier") || strings.Contains(impl, "quality_modifier"):
		return modifierToTitleRegex(value), negate
	case strings.Contains(impl, "language"):
		// Title-only scoring cannot treat "no EN token" as non-English — most
		// English releases omit it. Invert English+negate to a positive match
		// on other language tokens (GERMAN, FRENCH, …).
		if spec.Negate && strings.TrimSpace(value) == "1" {
			return nonEnglishLanguageToTitleRegex(), false
		}
		return languageToTitleRegex(value), negate
	case strings.Contains(impl, "indexerflag") || strings.Contains(impl, "indexer_flag"):
		return indexerFlagToTitleRegex(value), negate
	default:
		return "", negate
	}
}

func specToTitleRegex(spec trashSpec) string {
	rx, _ := specToTitleMatch(spec)
	return rx
}

func parseTrashFormat(raw []byte, scoreSet string) (*formatsv1.CustomFormat, error) {
	var in trashCFFile
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.TrashID) == "" {
		return nil, fmt.Errorf("missing name or trash_id")
	}
	var required []*formatsv1.FormatRule
	var optional []string
	for _, spec := range in.Specifications {
		rx, negate := specToTitleMatch(spec)
		if rx == "" {
			continue
		}
		if spec.Required {
			required = append(required, &formatsv1.FormatRule{
				Field: "title", Op: "matches", Value: rx, Negate: negate,
			})
			continue
		}
		if negate {
			required = append(required, &formatsv1.FormatRule{
				Field: "title", Op: "matches", Value: rx, Negate: true,
			})
			continue
		}
		optional = append(optional, "(?:" + rx + ")")
	}
	if len(optional) > 0 {
		required = append(required, &formatsv1.FormatRule{
			Field: "title", Op: "matches", Value: strings.Join(optional, "|"),
		})
	}
	if len(required) == 0 {
		return nil, fmt.Errorf("no matchable specifications")
	}
	return &formatsv1.CustomFormat{
		Id:           trashFormatID(in.TrashID),
		Name:         in.Name,
		Rules:        required,
		DefaultScore: trashScore(in.TrashScores, scoreSet),
	}, nil
}

func decodeFormatItems(raw json.RawMessage, scoreLookup map[string]int32) []trashFormatItem {
	if len(raw) == 0 {
		return nil
	}
	var items []trashFormatItem
	if err := json.Unmarshal(raw, &items); err == nil && len(items) > 0 {
		return items
	}
	var named map[string]string
	if err := json.Unmarshal(raw, &named); err == nil && len(named) > 0 {
		out := make([]trashFormatItem, 0, len(named))
		for name, id := range named {
			score := int32(0)
			if scoreLookup != nil {
				score = scoreLookup[id]
			}
			out = append(out, trashFormatItem{TrashID: id, Name: name, Score: score})
		}
		return out
	}
	return nil
}

func parseTrashProfile(raw []byte, scoreLookup map[string]int32) (*formatsv1.QualityProfile, error) {
	var in trashProfileFile
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("missing profile name")
	}
	id := in.TrashID
	if id == "" {
		id = in.Name
	}
	minScore := in.MinFormatScore
	if in.MinScore != 0 {
		minScore = in.MinScore
	}
	cutoff := in.CutoffFormatScore
	if in.CutoffScore != 0 {
		cutoff = in.CutoffScore
	}
	upgrade := true
	if in.UpgradeAllowed != nil {
		upgrade = *in.UpgradeAllowed
	} else if in.UpgradeAllowedAlt != nil {
		upgrade = *in.UpgradeAllowedAlt
	}
	items := decodeFormatItems(in.FormatItems, scoreLookup)
	if len(items) == 0 {
		items = decodeFormatItems(in.FormatItemsAlt, scoreLookup)
	}
	scores := map[string]int32{}
	for _, item := range items {
		fid := trashFormatID(item.TrashID)
		if fid == "" {
			continue
		}
		score := item.Score
		if score == 0 && scoreLookup != nil {
			if v, ok := scoreLookup[item.TrashID]; ok {
				score = v
			}
		}
		scores[fid] = score
	}
	return &formatsv1.QualityProfile{
		Id:             trashProfileID(id),
		Name:           in.Name,
		MinScore:       minScore,
		CutoffScore:    cutoff,
		UpgradeAllowed: upgrade,
		FormatScores:   scores,
	}, nil
}

func qualifyFormatName(path, name string) string {
	if strings.Contains(path, "/sonarr/") && !strings.HasSuffix(name, " (TV)") {
		return name + " (TV)"
	}
	return name
}

func serviceList(services []string) []string {
	if len(services) == 0 {
		return []string{"radarr", "sonarr"}
	}
	out := make([]string, 0, len(services))
	seen := map[string]bool{}
	for _, s := range services {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{"radarr", "sonarr"}
	}
	return out
}

func findGuidesJSONRoot(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	candidates := []string{
		filepath.Join(path, "docs", "json"),
		filepath.Join(path, "Guides-master", "docs", "json"),
		path,
	}
	for _, c := range candidates {
		if st, err := os.Stat(filepath.Join(c, "radarr", "cf")); err == nil && st.IsDir() {
			return c
		}
		if st, err := os.Stat(filepath.Join(c, "sonarr", "cf")); err == nil && st.IsDir() {
			return c
		}
	}
	var found string
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || found != "" {
			return err
		}
		if d.Name() != "json" {
			return nil
		}
		if filepath.Base(filepath.Dir(p)) != "docs" {
			return nil
		}
		if st, err := os.Stat(filepath.Join(p, "radarr", "cf")); err == nil && st.IsDir() {
			found = p
			return fs.SkipAll
		}
		if st, err := os.Stat(filepath.Join(p, "sonarr", "cf")); err == nil && st.IsDir() {
			found = p
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func walkJSON(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".json") {
			files = append(files, path)
		}
		return nil
	})
	return files
}

func collectServiceJSON(jsonRoot string, services []string, kinds ...string) []string {
	var files []string
	for _, svc := range services {
		for _, kind := range kinds {
			files = append(files, walkJSON(filepath.Join(jsonRoot, svc, kind))...)
		}
	}
	return files
}

func collectEmbeddedJSON(services []string, kinds ...string) []struct {
	name string
	data []byte
} {
	var out []struct {
		name string
		data []byte
	}
	_ = fs.WalkDir(bundledGuides, "guides-fixture/docs/json", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".json") {
			return nil
		}
		rel := strings.TrimPrefix(path, "guides-fixture/docs/json/")
		parts := strings.Split(rel, "/")
		if len(parts) < 3 {
			return nil
		}
		svc, kind := parts[0], parts[1]
		okSvc := false
		for _, s := range services {
			if s == svc {
				okSvc = true
				break
			}
		}
		if !okSvc {
			return nil
		}
		okKind := false
		for _, k := range kinds {
			if k == kind {
				okKind = true
				break
			}
		}
		if !okKind {
			return nil
		}
		data, err := fs.ReadFile(bundledGuides, path)
		if err != nil {
			return nil
		}
		out = append(out, struct {
			name string
			data []byte
		}{name: path, data: data})
		return nil
	})
	return out
}

func (m *Module) upsertFormat(f *formatsv1.CustomFormat) (upserted bool, err error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rulesJSON, _ := json.Marshal(f.GetRules())
	res, err := m.db.Exec(`
		INSERT INTO custom_formats (id, name, rules_json, default_score, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			rules_json=excluded.rules_json,
			default_score=excluded.default_score,
			updated_at=excluded.updated_at
	`, f.GetId(), f.GetName(), string(rulesJSON), f.GetDefaultScore(), now, now)
	if err != nil {
		// Unique name from a seed/manual row — update that row in place.
		_, err2 := m.db.Exec(`
			UPDATE custom_formats SET id=?, rules_json=?, default_score=?, updated_at=? WHERE name=?
		`, f.GetId(), string(rulesJSON), f.GetDefaultScore(), now, f.GetName())
		if err2 != nil {
			return false, err
		}
		return true, nil
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (m *Module) upsertProfile(p *formatsv1.QualityProfile) error {
	now := time.Now().UTC().Format(time.RFC3339)
	scoresJSON, _ := json.Marshal(p.GetFormatScores())
	_, err := m.db.Exec(`
		INSERT INTO quality_profiles (id, name, min_score, cutoff_score, upgrade_allowed, upgrade_delay_minutes, format_scores_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			min_score=excluded.min_score,
			cutoff_score=excluded.cutoff_score,
			upgrade_allowed=excluded.upgrade_allowed,
			format_scores_json=excluded.format_scores_json,
			updated_at=excluded.updated_at
	`, p.GetId(), p.GetName(), p.GetMinScore(), p.GetCutoffScore(), boolToInt(p.GetUpgradeAllowed()), string(scoresJSON), now, now)
	if err != nil {
		_, err2 := m.db.Exec(`
			UPDATE quality_profiles SET id=?, min_score=?, cutoff_score=?, upgrade_allowed=?, format_scores_json=?, updated_at=? WHERE name=?
		`, p.GetId(), p.GetMinScore(), p.GetCutoffScore(), boolToInt(p.GetUpgradeAllowed()), string(scoresJSON), now, p.GetName())
		return err2
	}
	return nil
}

func (m *Module) SyncTrashGuides(ctx context.Context, req *formatsv1.SyncTrashGuidesRequest) (*formatsv1.SyncTrashGuidesResponse, error) {
	scoreSet := strings.TrimSpace(req.GetScoreSet())
	if scoreSet == "" {
		scoreSet = envOr("FORMATS_TRASH_SCORE_SET", "default")
	}
	services := serviceList(req.GetServices())
	if env := strings.TrimSpace(os.Getenv("FORMATS_TRASH_SERVICES")); len(req.GetServices()) == 0 && env != "" {
		services = serviceList(strings.Split(env, ","))
	}
	importProfiles := req.GetImportProfiles()

	guidesPath := strings.TrimSpace(req.GetGuidesPath())
	if guidesPath == "" {
		guidesPath = strings.TrimSpace(os.Getenv("FORMATS_TRASH_GUIDES_PATH"))
	}
	if isOfficialGuidesSentinel(guidesPath) {
		resolved, err := m.fetchOfficialTrashGuides(ctx)
		if err != nil {
			return nil, fmt.Errorf("official TRaSH Guides: %w", err)
		}
		guidesPath = resolved
	}

	resp := &formatsv1.SyncTrashGuidesResponse{}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}

	scoreLookup := map[string]int32{}
	rememberScore := func(raw []byte, f *formatsv1.CustomFormat) {
		var in trashCFFile
		if json.Unmarshal(raw, &in) == nil && in.TrashID != "" {
			scoreLookup[in.TrashID] = f.GetDefaultScore()
		}
	}

	jsonRoot := findGuidesJSONRoot(guidesPath)
	if jsonRoot != "" {
		resp.GuidesPath = jsonRoot
		for _, path := range collectServiceJSON(jsonRoot, services, "cf") {
			raw, err := os.ReadFile(path)
			if err != nil {
				resp.Warnings = append(resp.Warnings, path+": "+err.Error())
				resp.FormatsSkipped++
				continue
			}
			f, err := parseTrashFormat(raw, scoreSet)
			if err != nil {
				resp.FormatsSkipped++
				continue
			}
			f.Name = qualifyFormatName(path, f.GetName())
			if _, err := m.upsertFormat(f); err != nil {
				resp.Warnings = append(resp.Warnings, f.GetName()+": "+err.Error())
				resp.FormatsSkipped++
				continue
			}
			rememberScore(raw, f)
			resp.FormatsUpserted++
		}
		if importProfiles {
			for _, path := range collectServiceJSON(jsonRoot, services, "quality_profiles", "quality-profiles") {
				raw, err := os.ReadFile(path)
				if err != nil {
					resp.Warnings = append(resp.Warnings, path+": "+err.Error())
					continue
				}
				p, err := parseTrashProfile(raw, scoreLookup)
				if err != nil {
					continue
				}
				if err := m.upsertProfile(p); err != nil {
					resp.Warnings = append(resp.Warnings, p.GetName()+": "+err.Error())
					continue
				}
				resp.ProfilesUpserted++
			}
		}
		return resp, nil
	}

	resp.GuidesPath = "embedded:guides-fixture"
	for _, file := range collectEmbeddedJSON(services, "cf") {
		f, err := parseTrashFormat(file.data, scoreSet)
		if err != nil {
			resp.FormatsSkipped++
			continue
		}
		f.Name = qualifyFormatName(file.name, f.GetName())
		if _, err := m.upsertFormat(f); err != nil {
			resp.Warnings = append(resp.Warnings, f.GetName()+": "+err.Error())
			resp.FormatsSkipped++
			continue
		}
		rememberScore(file.data, f)
		resp.FormatsUpserted++
	}
	if importProfiles {
		for _, file := range collectEmbeddedJSON(services, "quality_profiles", "quality-profiles") {
			p, err := parseTrashProfile(file.data, scoreLookup)
			if err != nil {
				continue
			}
			if err := m.upsertProfile(p); err != nil {
				resp.Warnings = append(resp.Warnings, p.GetName()+": "+err.Error())
				continue
			}
			resp.ProfilesUpserted++
		}
	}
	return resp, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envTruthy(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

func (m *Module) maybeSyncTrashOnStart(ctx context.Context) {
	flag := strings.ToLower(strings.TrimSpace(os.Getenv("FORMATS_TRASH_SYNC")))
	if flag != "true" && flag != "1" && flag != "yes" && flag != "on" {
		return
	}
	importProfiles := os.Getenv("FORMATS_TRASH_IMPORT_PROFILES") != "false"
	guidesPath := strings.TrimSpace(os.Getenv("FORMATS_TRASH_GUIDES_PATH"))
	if isOfficialGuidesSentinel(guidesPath) || envTruthy("FORMATS_TRASH_OFFICIAL") {
		guidesPath = officialTrashGuidesSentinel
	}
	resp, err := m.SyncTrashGuides(ctx, &formatsv1.SyncTrashGuidesRequest{
		ScoreSet:       envOr("FORMATS_TRASH_SCORE_SET", "default"),
		ImportProfiles: importProfiles,
		Services:       serviceList(strings.Split(envOr("FORMATS_TRASH_SERVICES", "radarr,sonarr"), ",")),
		GuidesPath:     guidesPath,
	})
	if err != nil {
		return
	}
	_ = resp
}
