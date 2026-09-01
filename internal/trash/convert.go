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
	case "SizeSpecification":
		var sizeFields SizeSpecFields
		if err := json.Unmarshal(spec.Fields, &sizeFields); err != nil {
			return nil, false, "invalid size fields"
		}
		if sizeFields.Min > 0 {
			rule.Field = "size"
			rule.Op = "gt"
			rule.Value = strconv.FormatInt(int64(sizeFields.Min*1e9), 10)
			return rule, true, ""
		}
		if sizeFields.Max > 0 {
			rule.Field = "size"
			rule.Op = "lte"
			rule.Value = strconv.FormatInt(int64(sizeFields.Max*1e9), 10)
			return rule, true, ""
		}
		return nil, false, "size min/max not set"
	case "YearSpecification":
		if val == "" {
			return nil, false, "empty year"
		}
		rule.Field = "title"
		rule.Op = "matches"
		rule.Value = `(?i)\b` + regexpQuote(val) + `\b`
		return rule, true, ""
	case "EditionSpecification":
		if val == "" {
			return nil, false, "empty edition pattern"
		}
		rule.Field = "title"
		rule.Op = "matches"
		if strings.HasPrefix(val, "(?") {
			rule.Value = val
		} else {
			rule.Value = `(?i)\b` + regexpQuote(val) + `\b`
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

func regexpQuote(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '.', '+', '*', '?', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\':
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
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
	case "3": // Spanish (Latin)
		return `(?i)\b(spanish|espanol|castellano|latino)\b`
	case "4": // German
		return `(?i)\b(german|deutsch|dl|german.?dl)\b`
	case "5": // Italian
		return `(?i)\b(italian|ita|italiano)\b`
	case "6": // Korean
		return `(?i)\b(korean|kor|hangul)\b`
	case "7": // Chinese
		return `(?i)\b(chinese|chi|mandarin|cantonese)\b`
	case "8": // Japanese
		return `(?i)\b(japanese|jap|jpn)\b`
	case "9": // Portuguese
		return `(?i)\b(portuguese|portugues|pt-?br|pt-?pt)\b`
	case "10": // Spanish
		return `(?i)\b(spanish|espanol|castellano|latino)\b`
	case "11": // Polish
		return `(?i)\b(polish|pol|polski)\b`
	case "12": // Russian
		return `(?i)\b(russian|rus|russkiy)\b`
	case "13": // Arabic
		return `(?i)\b(arabic|ara)\b`
	case "14": // Hindi
		return `(?i)\b(hindi|hin)\b`
	case "15": // Turkish
		return `(?i)\b(turkish|tur|turkce)\b`
	case "16": // Danish
		return `(?i)\b(danish|dan|dansk)\b`
	case "17": // Finnish
		return `(?i)\b(finnish|fin|suomi)\b`
	case "18": // Swedish
		return `(?i)\b(swedish|swe|svenska)\b`
	case "19": // Norwegian
		return `(?i)\b(norwegian|nor|norsk)\b`
	case "20": // Czech
		return `(?i)\b(czech|cze|cesky)\b`
	case "21": // Dutch
		return `(?i)\b(dutch|nld|nl|nederlands)\b`
	case "22": // Romanian
		return `(?i)\b(romanian|rom|romana)\b`
	case "23": // Bulgarian
		return `(?i)\b(bulgarian|bul)\b`
	case "24": // Greek
		return `(?i)\b(greek|gre|ellinika)\b`
	case "25": // Hungarian
		return `(?i)\b(hungarian|hun|magyar)\b`
	case "26": // Hebrew
		return `(?i)\b(hebrew|heb)\b`
	case "27": // Thai
		return `(?i)\b(thai|tha)\b`
	case "28": // Vietnamese
		return `(?i)\b(vietnamese|vie)\b`
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

// FlattenQualityItems expands nested TRaSH profile items into a flat allowed map.
func FlattenQualityItems(items []QualityProfileItem) map[string]bool {
	out := make(map[string]bool)
	for _, item := range items {
		if len(item.Items) > 0 {
			for _, sub := range item.Items {
				out[sub] = item.Allowed
			}
		}
		if item.Name != "" {
			out[item.Name] = item.Allowed
		}
	}
	return out
}
