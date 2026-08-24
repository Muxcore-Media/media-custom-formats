package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
	"google.golang.org/grpc"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"

	"github.com/Muxcore-Media/core/pkg/contracts"
	_ "modernc.org/sqlite"
)

type Module struct {
	formatsv1.UnimplementedFormatServiceServer

	mu sync.RWMutex
	db *sql.DB

	id           string
	dbPath       string
	grpcAddr     string
	seedDefaults bool
	grpcSrv      *grpc.Server
	grpcLis      net.Listener

	trashGuidesPath     string
	trashGuidesURL      string
	trashCacheDir       string
	trashSyncOnStart    bool
	trashScoreSet       string
	trashImportProfiles bool
	trashServices       []string
}

type Config struct {
	ID           string
	DBPath       string
	GRPCAddr     string
	SeedDefaults bool

	TrashGuidesPath     string
	TrashGuidesURL      string
	TrashCacheDir       string
	TrashSyncOnStart    bool
	TrashScoreSet       string
	TrashImportProfiles bool
	TrashServices       []string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "media-custom-formats"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "/var/lib/media-custom-formats/formats.db"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9490"
	}
	if v := os.Getenv("FORMATS_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("FORMATS_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if os.Getenv("FORMATS_SEED_DEFAULTS") == "true" {
		cfg.SeedDefaults = true
	}
	if v := os.Getenv("FORMATS_TRASH_GUIDES_PATH"); v != "" {
		cfg.TrashGuidesPath = v
	}
	if v := os.Getenv("FORMATS_TRASH_GUIDES_URL"); v != "" {
		cfg.TrashGuidesURL = v
	}
	if v := os.Getenv("FORMATS_TRASH_CACHE_DIR"); v != "" {
		cfg.TrashCacheDir = v
	}
	if os.Getenv("FORMATS_TRASH_SYNC") == "true" {
		cfg.TrashSyncOnStart = true
	}
	if v := os.Getenv("FORMATS_TRASH_SCORE_SET"); v != "" {
		cfg.TrashScoreSet = v
	}
	if os.Getenv("FORMATS_TRASH_IMPORT_PROFILES") == "true" {
		cfg.TrashImportProfiles = true
	}
	if v := os.Getenv("FORMATS_TRASH_SERVICES"); v != "" {
		cfg.TrashServices = splitCSV(v)
	}
	if cfg.TrashScoreSet == "" {
		cfg.TrashScoreSet = "default"
	}
	if cfg.TrashCacheDir == "" {
		cfg.TrashCacheDir = "/var/lib/media-custom-formats/trash-guides"
	}
	return &Module{
		id:                  cfg.ID,
		dbPath:              cfg.DBPath,
		grpcAddr:            cfg.GRPCAddr,
		seedDefaults:        cfg.SeedDefaults,
		trashGuidesPath:     cfg.TrashGuidesPath,
		trashGuidesURL:      cfg.TrashGuidesURL,
		trashCacheDir:       cfg.TrashCacheDir,
		trashSyncOnStart:    cfg.TrashSyncOnStart,
		trashScoreSet:       cfg.TrashScoreSet,
		trashImportProfiles: cfg.TrashImportProfiles,
		trashServices:       cfg.TrashServices,
	}
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:             m.id,
		Name:           "Custom Formats",
		Version:        "0.1.9",
		Roles:          []string{"scoring"},
		Description:    "Custom format definitions, quality profiles, and release scoring engine",
		Author:         "MuxCore",
		Capabilities:   []string{"media.scoring", "media.formats", "settings"},
		MinCoreVersion: "0.4.0",
		HTTPAddr:       m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	dir := filepath.Dir(m.dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", m.dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS custom_formats (
			id            TEXT PRIMARY KEY,
			name          TEXT NOT NULL UNIQUE,
			rules_json    TEXT NOT NULL DEFAULT '[]',
			default_score INTEGER NOT NULL DEFAULT 0,
			trash_id      TEXT NOT NULL DEFAULT '',
			trash_service TEXT NOT NULL DEFAULT '',
			created_at    TEXT NOT NULL,
			updated_at    TEXT NOT NULL
		)
	`); err != nil {
		_ = db.Close()
		return fmt.Errorf("create custom_formats table: %w", err)
	}
	for _, col := range []string{
		`ALTER TABLE custom_formats ADD COLUMN trash_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE custom_formats ADD COLUMN trash_service TEXT NOT NULL DEFAULT ''`,
	} {
		_, _ = db.ExecContext(ctx, col) // ignore if already present
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS quality_profiles (
			id                  TEXT PRIMARY KEY,
			name                TEXT NOT NULL UNIQUE,
			min_score           INTEGER NOT NULL DEFAULT 0,
			cutoff_score         INTEGER NOT NULL DEFAULT 0,
			upgrade_allowed     INTEGER NOT NULL DEFAULT 1,
			upgrade_delay_minutes INTEGER NOT NULL DEFAULT 0,
			format_scores_json  TEXT NOT NULL DEFAULT '{}',
			created_at          TEXT NOT NULL,
			updated_at          TEXT NOT NULL
		)
	`); err != nil {
		_ = db.Close()
		return fmt.Errorf("create quality_profiles table: %w", err)
	}
	if err := m.migrateReleaseGroups(ctx, db); err != nil {
		_ = db.Close()
		return fmt.Errorf("create release_profile_groups: %w", err)
	}

	m.mu.Lock()
	m.db = db
	m.mu.Unlock()

	synced := false
	if m.trashSyncOnStart {
		synced = m.maybeSyncTrashOnStart(ctx)
	}
	// Lightweight seeds when trash sync is off, or as fallback if sync failed.
	if m.seedDefaults && !synced {
		m.seedDefaultFormats(ctx)
	}
	if m.seedDefaults {
		m.seedDefaultReleaseGroups(ctx)
	}

	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.grpcLis = lis

	slog.Info("media-custom-formats initialized", "db", m.dbPath, "grpc", m.grpcAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	formatsv1.RegisterFormatServiceServer(m.grpcSrv, m)
	m.registerSettingsMesh(m.grpcSrv)

	go func() {
		slog.Info("media-custom-formats gRPC started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.grpcLis); err != nil {
			slog.Error("media-custom-formats gRPC error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	m.mu.Lock()
	if m.db != nil {
		_ = m.db.Close()
		m.db = nil
	}
	m.mu.Unlock()
	slog.Info("media-custom-formats stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	m.mu.RLock()
	db := m.db
	m.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("not initialized")
	}
	return db.PingContext(ctx)
}

// ── Custom Format CRUD ─────────────────────────────────────────

func (m *Module) ListFormats(ctx context.Context, req *formatsv1.ListFormatsRequest) (*formatsv1.ListFormatsResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &formatsv1.ListFormatsResponse{Formats: m.loadFormats()}, nil
}

func (m *Module) CreateFormat(ctx context.Context, req *formatsv1.CreateFormatRequest) (*formatsv1.CreateFormatResponse, error) {
	if req.GetName() == "" {
		return nil, fmt.Errorf("name is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	id := fmt.Sprintf("cf_%d", time.Now().UnixNano())
	rulesJSON, _ := json.Marshal(req.GetRules())

	_, err := m.db.Exec(`INSERT INTO custom_formats (id, name, rules_json, default_score, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, req.GetName(), string(rulesJSON), req.GetDefaultScore(), now, now)
	if err != nil {
		return nil, fmt.Errorf("create format: %w", err)
	}

	return &formatsv1.CreateFormatResponse{Format: m.loadFormat(id)}, nil
}

func (m *Module) UpdateFormat(ctx context.Context, req *formatsv1.UpdateFormatRequest) (*formatsv1.UpdateFormatResponse, error) {
	if req.GetId() == "" {
		return nil, fmt.Errorf("id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	rulesJSON, _ := json.Marshal(req.GetRules())

	_, err := m.db.Exec(`UPDATE custom_formats SET name=?, rules_json=?, default_score=?, updated_at=? WHERE id=?`,
		req.GetName(), string(rulesJSON), req.GetDefaultScore(), now, req.GetId())
	if err != nil {
		return nil, fmt.Errorf("update format: %w", err)
	}

	f := m.loadFormat(req.GetId())
	if f == nil {
		return nil, fmt.Errorf("format not found: %s", req.GetId())
	}
	return &formatsv1.UpdateFormatResponse{Format: f}, nil
}

func (m *Module) DeleteFormat(ctx context.Context, req *formatsv1.DeleteFormatRequest) (*formatsv1.DeleteFormatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`DELETE FROM custom_formats WHERE id = ?`, req.GetId())
	return &formatsv1.DeleteFormatResponse{}, err
}

// ── Quality Profile CRUD ───────────────────────────────────────

func (m *Module) ListProfiles(ctx context.Context, req *formatsv1.ListProfilesRequest) (*formatsv1.ListProfilesResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &formatsv1.ListProfilesResponse{Profiles: m.loadProfiles()}, nil
}

func (m *Module) CreateProfile(ctx context.Context, req *formatsv1.CreateProfileRequest) (*formatsv1.CreateProfileResponse, error) {
	if req.GetName() == "" {
		return nil, fmt.Errorf("name is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	id := fmt.Sprintf("qp_%d", time.Now().UnixNano())
	scoresJSON, _ := json.Marshal(req.GetFormatScores())

	_, err := m.db.Exec(`INSERT INTO quality_profiles (id, name, min_score, cutoff_score, upgrade_allowed, upgrade_delay_minutes, format_scores_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.GetName(), req.GetMinScore(), req.GetCutoffScore(), boolToInt(req.GetUpgradeAllowed()), req.GetUpgradeDelayMinutes(), string(scoresJSON), now, now)
	if err != nil {
		return nil, fmt.Errorf("create profile: %w", err)
	}

	return &formatsv1.CreateProfileResponse{Profile: m.loadProfile(id)}, nil
}

func (m *Module) UpdateProfile(ctx context.Context, req *formatsv1.UpdateProfileRequest) (*formatsv1.UpdateProfileResponse, error) {
	if req.GetId() == "" {
		return nil, fmt.Errorf("id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	scoresJSON, _ := json.Marshal(req.GetFormatScores())

	_, err := m.db.Exec(`UPDATE quality_profiles SET name=?, min_score=?, cutoff_score=?, upgrade_allowed=?, upgrade_delay_minutes=?, format_scores_json=?, updated_at=? WHERE id=?`,
		req.GetName(), req.GetMinScore(), req.GetCutoffScore(), boolToInt(req.GetUpgradeAllowed()), req.GetUpgradeDelayMinutes(), string(scoresJSON), now, req.GetId())
	if err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}

	p := m.loadProfile(req.GetId())
	if p == nil {
		return nil, fmt.Errorf("profile not found: %s", req.GetId())
	}
	return &formatsv1.UpdateProfileResponse{Profile: p}, nil
}

func (m *Module) DeleteProfile(ctx context.Context, req *formatsv1.DeleteProfileRequest) (*formatsv1.DeleteProfileResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`DELETE FROM quality_profiles WHERE id = ?`, req.GetId())
	return &formatsv1.DeleteProfileResponse{}, err
}

// ── Scoring Engine ─────────────────────────────────────────────

var (
	reResolution = regexp.MustCompile(`(?i)(\d{3,4})[pi]`)
	reRemux      = regexp.MustCompile(`(?i)remux`)
	reBluRay     = regexp.MustCompile(`(?i)bluray|brrip|bdrip|blu-ray|blu.?ray`)
	reBRDisk     = regexp.MustCompile(`(?i)\b(br[-\s]?disk|bd[-\s]?disk)\b`)
	reWebRip     = regexp.MustCompile(`(?i)web[-\s]?rip|webrip`)
	reWebDL      = regexp.MustCompile(`(?i)web[-\s]?dl|webdl`)
	reHDTV       = regexp.MustCompile(`(?i)hdtv|hd.?tv`)
	reCam        = regexp.MustCompile(`(?i)cam|ts|tc|hdts|hd-cam`)
	reRawHD      = regexp.MustCompile(`(?i)rawhd|raw.?hd`)
	reHDR        = regexp.MustCompile(`(?i)hdr\d*|dolby.?vision|dv|hlg|hdr10\+`)
	reCodecHEVC  = regexp.MustCompile(`(?i)hevc|h[. ]?265|x265`)
	reCodecAV1   = regexp.MustCompile(`(?i)av1`)
	reCodecVP9   = regexp.MustCompile(`(?i)vp[89]`)
	reCodecH264  = regexp.MustCompile(`(?i)h[. ]?264|x264|avc`)
)

func (m *Module) ParseQuality(ctx context.Context, req *formatsv1.ParseQualityRequest) (*formatsv1.ParseQualityResponse, error) {
	q := parseQualityFromTitle(req.GetTitle())
	return &formatsv1.ParseQualityResponse{Quality: q}, nil
}

func (m *Module) ScoreRelease(ctx context.Context, req *formatsv1.ScoreReleaseRequest) (*formatsv1.ScoreReleaseResponse, error) {
	quality := parseQualityFromTitle(req.GetTitle())

	m.mu.RLock()
	formats := m.loadFormats()
	profiles := m.loadProfiles()
	m.mu.RUnlock()

	var profile *formatsv1.QualityProfile
	if req.GetProfileId() != "" {
		for _, p := range profiles {
			if p.GetId() == req.GetProfileId() {
				profile = p
				break
			}
		}
	} else if len(profiles) > 0 {
		profile = profiles[0]
	}

	var formatMatches []*formatsv1.FormatMatch
	var totalFormatScore int32
	for _, f := range formats {
		if !trashServiceAllowed(f.GetTrashService(), req.GetCategory(), req.GetSubCategory()) {
			continue
		}
		matched, rule := matchFormat(f, req.GetTitle(), req.GetSize(), req.GetSeeders(), quality)
		if matched {
			score := f.GetDefaultScore()
			if profile != nil {
				if ps, ok := profile.GetFormatScores()[f.GetId()]; ok {
					score = ps
				}
			}
			totalFormatScore += score
			formatMatches = append(formatMatches, &formatsv1.FormatMatch{
				FormatId:    f.GetId(),
				FormatName:  f.GetName(),
				Score:       score,
				RuleMatched: rule,
			})
		}
	}

	totalScore := quality.GetScore() + totalFormatScore

	m.mu.RLock()
	groups := m.loadReleaseGroups()
	m.mu.RUnlock()
	scored, ok := applyReleaseGroups(req.GetTitle(), groups, totalScore)
	if !ok {
		return &formatsv1.ScoreReleaseResponse{
			TotalScore:    -100000,
			QualityScore:  quality.GetScore(),
			FormatScore:   totalFormatScore,
			FormatMatches: formatMatches,
			Quality:       quality,
		}, nil
	}

	return &formatsv1.ScoreReleaseResponse{
		TotalScore:    scored,
		QualityScore:  quality.GetScore(),
		FormatScore:   totalFormatScore,
		FormatMatches: formatMatches,
		Quality:       quality,
	}, nil
}

func parseQualityFromTitle(title string) *formatsv1.QualityInfo {
	q := &formatsv1.QualityInfo{}

	resolution := parseResolution(title)
	switch {
	case resolution >= 2160:
		q.Resolution = "2160p"
	case resolution >= 1080:
		q.Resolution = "1080p"
	case resolution >= 720:
		q.Resolution = "720p"
	case resolution >= 576:
		q.Resolution = "576p"
	default:
		q.Resolution = "SD"
	}

	switch {
	case reRemux.MatchString(title):
		q.Source = "Remux"
	case reBRDisk.MatchString(title):
		q.Source = "BR-DISK"
	case reBluRay.MatchString(title):
		q.Source = "BluRay"
	case reRawHD.MatchString(title):
		q.Source = "RawHD"
	case reWebRip.MatchString(title):
		q.Source = "WEBRip"
	case reWebDL.MatchString(title):
		q.Source = "WEB-DL"
	case reHDTV.MatchString(title):
		q.Source = "HDTV"
	case reCam.MatchString(title):
		q.Source = "CAM"
	default:
		q.Source = "WEB-DL"
	}

	switch {
	case reCodecAV1.MatchString(title):
		q.Codec = "av1"
	case reCodecHEVC.MatchString(title):
		q.Codec = "hevc"
	case reCodecVP9.MatchString(title):
		q.Codec = "vp9"
	case reCodecH264.MatchString(title):
		q.Codec = "h264"
	}

	q.Hdr = reHDR.MatchString(title)

	q.Score = int32(qualityScore(q.Resolution, q.Source, q.Hdr))
	q.Label = qualityLabel(q.Resolution, q.Source, q.Hdr, q.Codec)
	return q
}

func parseResolution(title string) int {
	m := reResolution.FindStringSubmatch(title)
	if len(m) > 1 {
		if v, err := strconv.Atoi(m[1]); err == nil {
			return v
		}
	}
	lower := strings.ToLower(title)
	if strings.Contains(lower, "4k") || strings.Contains(lower, "uhd") {
		return 2160
	}
	if strings.Contains(lower, "1080") || strings.Contains(lower, "hd") {
		return 1080
	}
	return 0
}

func qualityScore(resolution, source string, hdr bool) int {
	score := 0
	switch resolution {
	case "2160p":
		score += 120
	case "1080p":
		score += 100
	case "720p":
		score += 80
	case "576p":
		score += 60
	default:
		score += 40
	}
	switch source {
	case "Remux":
		score += 40
	case "BluRay":
		score += 30
	case "RawHD":
		score += 25
	case "WEB-DL", "WEBRip":
		score += 20
	case "HDTV":
		score += 10
	case "BR-DISK":
		score -= 50
	case "CAM":
		score -= 100
	}
	if hdr {
		score += 10
	}
	return score
}

func qualityLabel(resolution, source string, hdr bool, codec string) string {
	parts := []string{resolution}
	if source != "" && source != "WEB-DL" {
		parts = append(parts, source)
	} else {
		parts = append(parts, "WEB-DL")
	}
	if hdr {
		parts = append(parts, "HDR")
	}
	if codec != "" {
		parts = append(parts, strings.ToUpper(codec))
	}
	return strings.Join(parts, " ")
}

func trashServiceAllowed(service, category, subCategory string) bool {
	if service == "" {
		return true
	}
	cat := strings.ToLower(strings.TrimSpace(category + " " + subCategory))
	if cat == "" {
		return true
	}
	switch service {
	case "radarr":
		if strings.Contains(cat, "tv") || strings.Contains(cat, "show") || strings.Contains(cat, "series") || strings.Contains(cat, "episode") {
			return false
		}
	case "sonarr":
		if strings.Contains(cat, "movie") || strings.Contains(cat, "film") {
			return false
		}
	}
	return true
}

func matchFormat(f *formatsv1.CustomFormat, title string, size int64, seeders int32, quality *formatsv1.QualityInfo) (bool, string) {
	rules := f.GetRules()
	if len(rules) == 0 {
		return true, ""
	}

	var required, optional []*formatsv1.FormatRule
	for _, rule := range rules {
		if rule.GetRequired() {
			required = append(required, rule)
		} else {
			optional = append(optional, rule)
		}
	}
	// Legacy formats (no required flags): treat all rules as optional → any-match (prior OR behavior).
	if len(required) == 0 && len(optional) == 0 {
		optional = rules
	}

	var matchedRule string
	for _, rule := range required {
		ok, detail := matchRuleDetail(rule, title, size, seeders, quality)
		if !ok {
			return false, ""
		}
		if matchedRule == "" {
			matchedRule = detail
		}
	}
	if len(optional) > 0 {
		any := false
		for _, rule := range optional {
			ok, detail := matchRuleDetail(rule, title, size, seeders, quality)
			if ok {
				any = true
				if matchedRule == "" {
					matchedRule = detail
				}
				break
			}
		}
		if !any {
			return false, ""
		}
	}
	return true, matchedRule
}

func matchRuleDetail(rule *formatsv1.FormatRule, title string, size int64, seeders int32, quality *formatsv1.QualityInfo) (bool, string) {
	matched := matchRule(rule, title, size, seeders, quality)
	if matched {
		return true, rule.GetValue()
	}
	return false, ""
}

func matchRule(rule *formatsv1.FormatRule, title string, size int64, seeders int32, quality *formatsv1.QualityInfo) bool {
	matched := false
	switch rule.GetField() {
	case "title":
		switch rule.GetOp() {
		case "matches":
			matched = matchRegex(rule.GetValue(), title, true)
		case "contains":
			matched = strings.Contains(strings.ToLower(title), strings.ToLower(rule.GetValue()))
		}
	case "release_group":
		grp := extractReleaseGroup(title)
		switch rule.GetOp() {
		case "matches":
			matched = matchRegex(rule.GetValue(), grp, false)
		case "contains", "eq":
			matched = strings.EqualFold(grp, rule.GetValue()) || matchRegex(rule.GetValue(), grp, false)
		}
	case "resolution":
		want, err := strconv.Atoi(strings.TrimSpace(rule.GetValue()))
		got := 0
		if quality != nil {
			got = resolutionNumber(quality.GetResolution())
		}
		if got == 0 {
			got = parseResolution(title)
		}
		if err == nil {
			switch rule.GetOp() {
			case "eq", "":
				matched = got == want
			case "gt":
				matched = got > want
			case "lt":
				matched = got < want
			case "gte":
				matched = got >= want
			case "lte":
				matched = got <= want
			}
		}
	case "source":
		got := 0
		if quality != nil {
			got = sourceNumber(quality.GetSource())
		}
		want, err := strconv.Atoi(strings.TrimSpace(rule.GetValue()))
		if err == nil {
			matched = got == want
		}
	case "quality_modifier":
		got := 0
		if quality != nil {
			got = qualityModifierNumber(quality.GetSource())
		}
		want, err := strconv.Atoi(strings.TrimSpace(rule.GetValue()))
		if err == nil {
			matched = got == want
		}
	case "size":
		threshold, err := strconv.ParseInt(rule.GetValue(), 10, 64)
		if err == nil {
			switch rule.GetOp() {
			case "gt":
				matched = size > threshold
			case "lt":
				matched = size < threshold
			case "gte":
				matched = size >= threshold
			case "lte":
				matched = size <= threshold
			}
		}
	case "seeders":
		threshold, err := strconv.ParseInt(rule.GetValue(), 10, 32)
		if err == nil {
			switch rule.GetOp() {
			case "gt":
				matched = int64(seeders) > threshold
			case "lt":
				matched = int64(seeders) < threshold
			case "gte":
				matched = int64(seeders) >= threshold
			case "lte":
				matched = int64(seeders) <= threshold
			}
		}
	}
	if rule.GetNegate() {
		return !matched
	}
	return matched
}

func matchRegex(pattern, text string, ignoreCase bool) bool {
	if pattern == "" {
		return false
	}
	re2pat := pattern
	if ignoreCase && !strings.HasPrefix(pattern, "(?i)") && !strings.HasPrefix(pattern, "(?I)") {
		re2pat = "(?i)" + pattern
	}
	if re, err := regexp.Compile(re2pat); err == nil {
		return re.MatchString(text)
	}
	var opts regexp2.RegexOptions
	if ignoreCase {
		opts = regexp2.IgnoreCase
	}
	re, err := regexp2.Compile(pattern, opts)
	if err != nil {
		return false
	}
	ok, _ := re.MatchString(text)
	return ok
}

func extractReleaseGroup(title string) string {
	base := title
	if i := strings.LastIndex(base, "."); i > 0 {
		ext := strings.ToLower(base[i+1:])
		if len(ext) <= 4 && !strings.ContainsAny(ext, " .-_") {
			base = base[:i]
		}
	}
	if i := strings.LastIndex(base, "-"); i >= 0 && i < len(base)-1 {
		return base[i+1:]
	}
	return base
}

func resolutionNumber(label string) int {
	switch strings.ToLower(label) {
	case "2160p":
		return 2160
	case "1080p":
		return 1080
	case "720p":
		return 720
	case "576p":
		return 576
	case "480p":
		return 480
	default:
		return 0
	}
}

// Radarr Source enum (subset used by TRaSH Guides).
func sourceNumber(source string) int {
	switch source {
	case "CAM":
		return 1
	case "DVD":
		return 5
	case "HDTV", "TV":
		return 6
	case "WEB-DL":
		return 7
	case "WEBRip":
		return 8
	case "BluRay", "Remux", "BR-DISK":
		return 9
	default:
		return 0
	}
}

// Radarr QualityModifier: Remux=5, BRDISK=4, RAWHD=3, …
func qualityModifierNumber(source string) int {
	switch source {
	case "Remux":
		return 5
	case "BR-DISK":
		return 4
	case "RawHD":
		return 3
	default:
		return 0
	}
}

// ── DB load helpers ────────────────────────────────────────────

func (m *Module) loadFormats() []*formatsv1.CustomFormat {
	rows, err := m.db.Query(`SELECT id, name, rules_json, default_score, created_at, updated_at, trash_id, trash_service FROM custom_formats ORDER BY name`)
	if err != nil {
		// Older DBs without trash columns
		rows, err = m.db.Query(`SELECT id, name, rules_json, default_score, created_at, updated_at FROM custom_formats ORDER BY name`)
		if err != nil {
			return nil
		}
		defer func() { _ = rows.Close() }()
		var formats []*formatsv1.CustomFormat
		for rows.Next() {
			f := scanFormatLegacy(rows)
			if f != nil {
				formats = append(formats, f)
			}
		}
		return formats
	}
	defer func() { _ = rows.Close() }()

	var formats []*formatsv1.CustomFormat
	for rows.Next() {
		f := scanFormat(rows)
		if f != nil {
			formats = append(formats, f)
		}
	}
	return formats
}

func (m *Module) loadFormat(id string) *formatsv1.CustomFormat {
	row := m.db.QueryRow(`SELECT id, name, rules_json, default_score, created_at, updated_at, trash_id, trash_service FROM custom_formats WHERE id = ?`, id)
	if f := scanFormatRow(row); f != nil {
		return f
	}
	row = m.db.QueryRow(`SELECT id, name, rules_json, default_score, created_at, updated_at FROM custom_formats WHERE id = ?`, id)
	return scanFormatRowLegacy(row)
}

func scanFormat(rows *sql.Rows) *formatsv1.CustomFormat {
	var id, name, rulesJSON, createdAt, updatedAt, trashID, trashService string
	var defaultScore int64
	if err := rows.Scan(&id, &name, &rulesJSON, &defaultScore, &createdAt, &updatedAt, &trashID, &trashService); err != nil {
		return nil
	}
	f := &formatsv1.CustomFormat{
		Id: id, Name: name, DefaultScore: int32(defaultScore),
		CreatedAt: createdAt, UpdatedAt: updatedAt,
		TrashId: trashID, TrashService: trashService,
	}
	_ = json.Unmarshal([]byte(rulesJSON), &f.Rules)
	return f
}

func scanFormatLegacy(rows *sql.Rows) *formatsv1.CustomFormat {
	var id, name, rulesJSON, createdAt, updatedAt string
	var defaultScore int64
	if err := rows.Scan(&id, &name, &rulesJSON, &defaultScore, &createdAt, &updatedAt); err != nil {
		return nil
	}
	f := &formatsv1.CustomFormat{
		Id: id, Name: name, DefaultScore: int32(defaultScore),
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	_ = json.Unmarshal([]byte(rulesJSON), &f.Rules)
	return f
}

func scanFormatRow(row *sql.Row) *formatsv1.CustomFormat {
	var id, name, rulesJSON, createdAt, updatedAt, trashID, trashService string
	var defaultScore int64
	if err := row.Scan(&id, &name, &rulesJSON, &defaultScore, &createdAt, &updatedAt, &trashID, &trashService); err != nil {
		return nil
	}
	f := &formatsv1.CustomFormat{
		Id: id, Name: name, DefaultScore: int32(defaultScore),
		CreatedAt: createdAt, UpdatedAt: updatedAt,
		TrashId: trashID, TrashService: trashService,
	}
	_ = json.Unmarshal([]byte(rulesJSON), &f.Rules)
	return f
}

func scanFormatRowLegacy(row *sql.Row) *formatsv1.CustomFormat {
	var id, name, rulesJSON, createdAt, updatedAt string
	var defaultScore int64
	if err := row.Scan(&id, &name, &rulesJSON, &defaultScore, &createdAt, &updatedAt); err != nil {
		return nil
	}
	f := &formatsv1.CustomFormat{
		Id: id, Name: name, DefaultScore: int32(defaultScore),
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	_ = json.Unmarshal([]byte(rulesJSON), &f.Rules)
	return f
}

func (m *Module) loadProfiles() []*formatsv1.QualityProfile {
	rows, err := m.db.Query(`SELECT id, name, min_score, cutoff_score, upgrade_allowed, upgrade_delay_minutes, format_scores_json, created_at, updated_at FROM quality_profiles ORDER BY name`)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var profiles []*formatsv1.QualityProfile
	for rows.Next() {
		p := scanProfile(rows)
		if p != nil {
			profiles = append(profiles, p)
		}
	}
	return profiles
}

func (m *Module) loadProfile(id string) *formatsv1.QualityProfile {
	row := m.db.QueryRow(`SELECT id, name, min_score, cutoff_score, upgrade_allowed, upgrade_delay_minutes, format_scores_json, created_at, updated_at FROM quality_profiles WHERE id = ?`, id)
	return scanProfileRow(row)
}

func scanProfile(rows *sql.Rows) *formatsv1.QualityProfile {
	var id, name, scoresJSON, createdAt, updatedAt string
	var minScore, cutoffScore, upgradeDelay int64
	var upgradeAllowed int
	if err := rows.Scan(&id, &name, &minScore, &cutoffScore, &upgradeAllowed, &upgradeDelay, &scoresJSON, &createdAt, &updatedAt); err != nil {
		return nil
	}
	p := &formatsv1.QualityProfile{
		Id: id, Name: name,
		MinScore: int32(minScore), CutoffScore: int32(cutoffScore),
		UpgradeAllowed: upgradeAllowed != 0, UpgradeDelayMinutes: int32(upgradeDelay),
		FormatScores: make(map[string]int32),
		CreatedAt:    createdAt, UpdatedAt: updatedAt,
	}
	_ = json.Unmarshal([]byte(scoresJSON), &p.FormatScores)
	return p
}

func scanProfileRow(row *sql.Row) *formatsv1.QualityProfile {
	var id, name, scoresJSON, createdAt, updatedAt string
	var minScore, cutoffScore, upgradeDelay int64
	var upgradeAllowed int
	if err := row.Scan(&id, &name, &minScore, &cutoffScore, &upgradeAllowed, &upgradeDelay, &scoresJSON, &createdAt, &updatedAt); err != nil {
		return nil
	}
	p := &formatsv1.QualityProfile{
		Id: id, Name: name,
		MinScore: int32(minScore), CutoffScore: int32(cutoffScore),
		UpgradeAllowed: upgradeAllowed != 0, UpgradeDelayMinutes: int32(upgradeDelay),
		FormatScores: make(map[string]int32),
		CreatedAt:    createdAt, UpdatedAt: updatedAt,
	}
	_ = json.Unmarshal([]byte(scoresJSON), &p.FormatScores)
	return p
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

var (
	_                  = math.Round
	_ contracts.Module = (*Module)(nil)
)
