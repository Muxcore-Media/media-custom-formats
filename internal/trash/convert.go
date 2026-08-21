package trash

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

// ConvertFormat maps a TRaSH custom format JSON into a MuxCore CustomFormat.
// Unsupported required specs cause skipped=true so callers can omit the format.
func ConvertFormat(cf CustomFormatJSON, service, scoreSet string) (out *formatsv1.CustomFormat, skipped bool, warn string) {
	if strings.TrimSpace(cf.TrashID) == "" || strings.TrimSpace(cf.Name) == "" {
		return nil, true, "missing trash_id or name"
	}
	if scoreSet == "" {
		scoreSet = "default"
	}
	score := 0
	if cf.TrashScores != nil {
		if v, ok := cf.TrashScores[scoreSet]; ok {
			score = v
		} else if v, ok := cf.TrashScores["default"]; ok {
			score = v
		}
	}

	var rules []*formatsv1.FormatRule
	var unsupportedRequired []string
	for _, spec := range cf.Specifications {
		rule, ok, reason := convertSpec(spec)
		if !ok {
			if spec.Required {
				unsupportedRequired = append(unsupportedRequired, fmt.Sprintf("%s (%s)", spec.Name, reason))
			}
			continue
		}
		rules = append(rules, rule)
	}
	if len(unsupportedRequired) > 0 {
		return nil, true, "unsupported required specs: " + strings.Join(unsupportedRequired, "; ")
	}
	if len(rules) == 0 && len(cf.Specifications) > 0 {
		return nil, true, "no convertible specifications"
	}

	return &formatsv1.CustomFormat{
		Id:           cf.TrashID,
		Name:         cf.Name,
		Rules:        rules,
		DefaultScore: int32(score),
		TrashId:      cf.TrashID,
		TrashService: service,
	}, false, ""
}

func convertSpec(spec Specification) (*formatsv1.FormatRule, bool, string) {
	var fields SpecFields
	_ = json.Unmarshal(spec.Fields, &fields)
	val := FieldValueString(fields.Value)

	rule := &formatsv1.FormatRule{
		Negate:   spec.Negate,
		Required: spec.Required,
		Op:       "eq",
		Value:    val,
	}

	switch spec.Implementation {
	case "ReleaseTitleSpecification":
		rule.Field = "title"
		rule.Op = "matches"
		if val == "" {
			return nil, false, "empty title pattern"
		}
		return rule, true, ""
	case "ReleaseGroupSpecification":
		rule.Field = "release_group"
		rule.Op = "matches"
		if val == "" {
			return nil, false, "empty release group pattern"
		}
		return rule, true, ""
	case "ResolutionSpecification":
		rule.Field = "resolution"
		rule.Op = "eq"
		if val == "" {
			return nil, false, "empty resolution"
		}
		return rule, true, ""
	case "SourceSpecification":
		rule.Field = "source"
		rule.Op = "eq"
		if val == "" {
			return nil, false, "empty source"
		}
		return rule, true, ""
	case "QualityModifierSpecification":
		rule.Field = "quality_modifier"
		rule.Op = "eq"
		if val == "" {
			return nil, false, "empty quality modifier"
		}
		return rule, true, ""
	case "LanguageSpecification":
		// ScoreRelease only sees the title; approximate via common language tokens.
		pat := languageTitlePattern(val)
		if pat == "" {
			return nil, false, "language id " + val + " not mapped"
		}
		rule.Field = "title"
		rule.Op = "matches"
		rule.Value = pat
		return rule, true, ""
	case "IndexerFlagSpecification", "ReleaseTypeSpecification":
		return nil, false, "unsupported " + spec.Implementation
	default:
		return nil, false, "unknown implementation " + spec.Implementation
	}
}

func languageTitlePattern(id string) string {
	// Radarr Language enum subset commonly used in TRaSH Guides.
	switch id {
	case "-2": // Original
		return ""
	case "1": // English
		return `(?i)\b(english|eng|en)\b`
	case "2": // French
		return `(?i)\b(french|fran[cç]ais|vff|vfq|truefrench|multi)\b`
	case "4": // German
		return `(?i)\b(german|deutsch|dl|german.?dl)\b`
	case "8": // Japanese
		return `(?i)\b(japanese|jap|jpn)\b`
	case "10": // Spanish
		return `(?i)\b(spanish|espanol|castellano|latino)\b`
	case "21": // Dutch
		return `(?i)\b(dutch|nld|nl)\b`
	default:
		return ""
	}
}

// ScoreForSet returns trash_scores[scoreSet] or default.
func ScoreForSet(scores map[string]int, scoreSet string) int {
	if scores == nil {
		return 0
	}
	if scoreSet == "" {
		scoreSet = "default"
	}
	if v, ok := scores[scoreSet]; ok {
		return v
	}
	return scores["default"]
}

// ParseIntValue parses a numeric rule value.
func ParseIntValue(v string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(v))
}
