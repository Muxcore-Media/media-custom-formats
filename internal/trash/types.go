// Package trash converts and syncs TRaSH Guides custom-format JSON into MuxCore rules.
package trash

import "encoding/json"

// Metadata is the Recyclarr-compatible metadata.json at the Guides repo root.
type Metadata struct {
	JSONPaths map[string]ServicePaths `json:"json_paths"`
}

// ServicePaths lists relative directories for each resource type.
type ServicePaths struct {
	CustomFormats        []string `json:"custom_formats"`
	Qualities            []string `json:"qualities"`
	Naming               []string `json:"naming"`
	QualityProfiles      []string `json:"quality_profiles"`
	CustomFormatGroups   []string `json:"custom_format_groups"`
	QualityProfileGroups []string `json:"quality_profile_groups"`
}

// CustomFormatJSON is one TRaSH / Arr custom format definition.
type CustomFormatJSON struct {
	TrashID                         string          `json:"trash_id"`
	TrashScores                     map[string]int  `json:"trash_scores"`
	Name                            string          `json:"name"`
	IncludeCustomFormatWhenRenaming bool            `json:"includeCustomFormatWhenRenaming"`
	Specifications                  []Specification `json:"specifications"`
}

// Specification is a single Arr custom-format condition.
type Specification struct {
	Name           string          `json:"name"`
	Implementation string          `json:"implementation"`
	Negate         bool            `json:"negate"`
	Required       bool            `json:"required"`
	Fields         json.RawMessage `json:"fields"`
}

// SpecFields holds the common fields.value payload (string or number).
type SpecFields struct {
	Value json.RawMessage `json:"value"`
}

// QualityProfileJSON is a TRaSH quality profile definition.
type QualityProfileJSON struct {
	TrashID           string            `json:"trash_id"`
	Name              string            `json:"name"`
	TrashScoreSet     string            `json:"trash_score_set"`
	UpgradeAllowed    bool              `json:"upgradeAllowed"`
	MinFormatScore    int               `json:"minFormatScore"`
	CutoffFormatScore int               `json:"cutoffFormatScore"`
	FormatItems       map[string]string `json:"formatItems"` // name → trash_id
}

// FieldValueString unwraps fields.value whether it is a JSON string or number.
func FieldValueString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return string(raw)
}
