package whois

import (
	"regexp"
	"strings"
)

var (
	urlPattern         = regexp.MustCompile(`https?://\S+`)
	bracketPattern     = regexp.MustCompile(`\[.*?\]`)
	parenPattern       = regexp.MustCompile(`\s*\(.*?\)\s*`)
	// Matches "key......: value" or "key: value" where key can have trailing dots
	dottedColonPattern = regexp.MustCompile(`^([a-zA-Z][a-zA-Z\s]*?)[\s.]*:\s*(.*)$`)
)

// ExtractRegistrar scans rawWhois looking for a field from the given list.
// It handles:
//   - Inline:   "Registrar: Foo Bar"
//   - Next-line: "Registrar:\n   Foo Bar"  (common in ccTLD responses)
//   - Dotted:   "registrar..........: Foo Bar"  (.fi format)
func ExtractRegistrar(rawWhois string, fields []string) string {
	lines := strings.Split(rawWhois, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		key, value := splitKeyValue(trimmed)
		if key == "" {
			continue
		}

		for _, field := range fields {
			wantKey := strings.TrimSuffix(strings.ToLower(field), ":")
			if strings.ToLower(key) != wantKey {
				continue
			}

			if value != "" {
				v := normalizeRegistrar(value)
				if v != "" {
					return v
				}
			}

			// Value might be on the next non-empty line (ccTLD pattern)
			for j := i + 1; j < len(lines) && j <= i+3; j++ {
				next := strings.TrimSpace(lines[j])
				if next == "" {
					continue
				}
				nextKey, _ := splitKeyValue(next)
				if nextKey != "" {
					break // next line is a new key
				}
				v := normalizeRegistrar(next)
				if v != "" {
					return v
				}
				break
			}
		}
	}
	return ""
}

// splitKeyValue parses a WHOIS line into key and value.
// Handles "Key: value", "Key: " and "key.......: value" formats.
// Returns ("", "") if the line doesn't look like a key-value pair.
func splitKeyValue(line string) (key, value string) {
	m := dottedColonPattern.FindStringSubmatch(line)
	if m == nil {
		return "", ""
	}
	return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
}

// looksLikeKey returns true if a line is a key-value line.
func looksLikeKey(line string) bool {
	k, _ := splitKeyValue(line)
	return k != ""
}

func normalizeRegistrar(s string) string {
	s = urlPattern.ReplaceAllString(s, "")
	s = bracketPattern.ReplaceAllString(s, "")
	s = parenPattern.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, ",;")
	s = strings.TrimSpace(s)
	return s
}
