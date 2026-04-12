package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wblech/wmux/internal/platform/pty"
)

func TestWithMaxSessions(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{}, WithMaxSessions(5))
	assert.Equal(t, 5, svc.maxSessions)
}

func TestWithMaxSessions_Zero(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{}, WithMaxSessions(0))
	assert.Equal(t, 0, svc.maxSessions)
}
