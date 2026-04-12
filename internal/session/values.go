package session

import "strings"

// ValidateSessionID checks that id is non-empty, contains only alphanumeric
// characters, hyphens, underscores, or forward slashes (for path prefixes),
// and does not contain path traversal sequences.
func ValidateSessionID(id string) error {
	if id == "" {
		return ErrInvalidSessionID
	}

	if strings.Contains(id, "..") {
		return ErrInvalidSessionID
	}

	for _, r := range id {
		if !isAllowedIDRune(r) {
			return ErrInvalidSessionID
		}
	}

	return nil
}

// isAllowedIDRune reports whether r is a character permitted in a session ID.
func isAllowedIDRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_' || r == '/'
}
