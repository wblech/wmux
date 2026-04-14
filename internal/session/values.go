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

// ExtractPrefix splits id at the last '/' and returns the prefix and name parts.
// If id contains no '/', prefix is empty and name equals the full id.
func ExtractPrefix(id string) (prefix, name string) {
	i := strings.LastIndex(id, "/")
	if i < 0 {
		return "", id
	}
	return id[:i], id[i+1:]
}

// ValidatePrefix checks that prefix is non-empty, at most 64 characters long,
// and contains only alphanumeric characters, hyphens, or underscores.
// Forward slashes are not allowed — they are the segment separator.
func ValidatePrefix(prefix string) error {
	if prefix == "" || len(prefix) > 64 {
		return ErrInvalidPrefix
	}
	for _, r := range prefix {
		if !isAllowedPrefixRune(r) {
			return ErrInvalidPrefix
		}
	}
	return nil
}

// isAllowedPrefixRune reports whether r is a character permitted in a prefix segment.
func isAllowedPrefixRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_'
}
