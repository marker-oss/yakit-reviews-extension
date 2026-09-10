package store

import "strings"

// AnonymizeAuthorName reduces a full name to first name + surname initial
// ("Анна Котова" -> "Анна К."). One-word and empty names are returned as-is.
// It is idempotent: an already-anonymized name is returned unchanged.
func AnonymizeAuthorName(full string) string {
	fields := strings.Fields(full)
	switch len(fields) {
	case 0:
		return ""
	case 1:
		return fields[0]
	}
	first := fields[0]
	second := fields[1]
	// Already anonymized ("X.") — leave it.
	if strings.HasSuffix(second, ".") {
		return first + " " + second
	}
	r := []rune(second)
	return first + " " + string(r[0]) + "."
}
