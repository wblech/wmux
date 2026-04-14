package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSessionID_Valid(t *testing.T) {
	valid := []string{
		"session-1",
		"my-project/session-1",
		"abc",
		"ABC_123",
		"a/b/c",
		"under_score",
	}

	for _, id := range valid {
		t.Run(id, func(t *testing.T) {
			require.NoError(t, ValidateSessionID(id))
		})
	}
}

func TestValidateSessionID_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"has spaces",
		"../traversal",
		"path/../traversal",
		"dot..dot",
		"with.dot",
		"with@special",
		"with#hash",
	}

	for _, id := range invalid {
		t.Run(id, func(t *testing.T) {
			assert.ErrorIs(t, ValidateSessionID(id), ErrInvalidSessionID)
		})
	}
}

func TestExtractPrefix(t *testing.T) {
	cases := []struct {
		id     string
		prefix string
		name   string
	}{
		{id: "my-session", prefix: "", name: "my-session"},
		{id: "proj-a/session-1", prefix: "proj-a", name: "session-1"},
		{id: "deep/nested/name", prefix: "deep/nested", name: "name"},
		{id: "a/b", prefix: "a", name: "b"},
		{id: "abc", prefix: "", name: "abc"},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			prefix, name := ExtractPrefix(tc.id)
			assert.Equal(t, tc.prefix, prefix)
			assert.Equal(t, tc.name, name)
		})
	}
}

func TestValidatePrefix_Valid(t *testing.T) {
	valid := []string{
		"proj-a",
		"my_prefix",
		"ABC123",
		"a-b-c",
	}

	for _, p := range valid {
		t.Run(p, func(t *testing.T) {
			require.NoError(t, ValidatePrefix(p))
		})
	}
}

func TestValidatePrefix_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"has spaces",
		"with.dot",
		"with@special",
		"with/slash",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", // 65 chars
	}

	for _, p := range invalid {
		t.Run(p, func(t *testing.T) {
			assert.ErrorIs(t, ValidatePrefix(p), ErrInvalidPrefix)
		})
	}
}

func TestValidateSessionID_WithPrefix(t *testing.T) {
	ids := []string{
		"proj/session-1",
		"org/team/session",
	}

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			require.NoError(t, ValidateSessionID(id))
		})
	}
}
