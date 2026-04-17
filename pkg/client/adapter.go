package client

import (
	"context"
	"fmt"
	"time"

	"github.com/wblech/wmux/internal/daemon"
	"github.com/wblech/wmux/internal/platform/protocol"
	"github.com/wblech/wmux/internal/session"
	"github.com/wblech/wmux/internal/transport"
)

// serverAdapter wraps *transport.Server to implement daemon.TransportServer.
type serverAdapter struct {
	srv *transport.Server
}

func (a *serverAdapter) OnClient(fn func(daemon.ConnectedClient)) {
	a.srv.OnClient(func(c *transport.Client) {
		fn(&clientAdapter{c: c})
	})
}

func (a *serverAdapter) Serve(ctx context.Context) error {
	if err := a.srv.Serve(ctx); err != nil {
		return fmt.Errorf("server adapter: serve: %w", err)
	}
	return nil
}

func (a *serverAdapter) BroadcastTo(clientID string, f protocol.Frame) error {
	if err := a.srv.BroadcastTo(clientID, f); err != nil {
		return fmt.Errorf("server adapter: broadcast: %w", err)
	}
	return nil
}

// clientAdapter wraps *transport.Client to implement daemon.ConnectedClient.
type clientAdapter struct {
	c *transport.Client
}

func (a *clientAdapter) ClientID() string {
	return a.c.ID
}

func (a *clientAdapter) Control() daemon.ControlConn {
	return a.c.Control
}

// sessionAdapter wraps *session.Service to implement daemon.SessionManager.
type sessionAdapter struct {
	svc *session.Service
}

func (a *sessionAdapter) Create(id string, opts daemon.SessionCreateOptions) (daemon.SessionInfo, error) {
	sess, err := a.svc.Create(id, session.CreateOptions{
		Shell:         opts.Shell,
		Args:          opts.Args,
		Cols:          opts.Cols,
		Rows:          opts.Rows,
		Cwd:           opts.Cwd,
		Env:           opts.Env,
		HighWatermark: 0,
		LowWatermark:  0,
		BatchInterval: 0,
		HistoryWriter: nil,
	})
	if err != nil {
		return daemon.SessionInfo{}, fmt.Errorf("session adapter: create: %w", err)
	}

	return toInfo(sess), nil
}

func (a *sessionAdapter) Get(id string) (daemon.SessionInfo, error) {
	sess, err := a.svc.Get(id)
	if err != nil {
		return daemon.SessionInfo{}, fmt.Errorf("session adapter: get: %w", err)
	}

	return toInfo(sess), nil
}

func (a *sessionAdapter) List() []daemon.SessionInfo {
	sessions := a.svc.List()
	infos := make([]daemon.SessionInfo, 0, len(sessions))

	for _, sess := range sessions {
		infos = append(infos, toInfo(sess))
	}

	return infos
}

func (a *sessionAdapter) Kill(id string) error {
	if err := a.svc.Kill(id); err != nil {
		return fmt.Errorf("session adapter: kill: %w", err)
	}
	return nil
}

func (a *sessionAdapter) Resize(id string, cols, rows int) error {
	if err := a.svc.Resize(id, cols, rows); err != nil {
		return fmt.Errorf("session adapter: resize: %w", err)
	}
	return nil
}

func (a *sessionAdapter) WriteInput(id string, data []byte) error {
	if err := a.svc.WriteInput(id, data); err != nil {
		return fmt.Errorf("session adapter: write input: %w", err)
	}
	return nil
}

func (a *sessionAdapter) ReadOutput(id string) ([]byte, error) {
	data, err := a.svc.ReadOutput(id)
	if err != nil {
		return nil, fmt.Errorf("session adapter: read output: %w", err)
	}
	return data, nil
}

func (a *sessionAdapter) Attach(id string) error {
	if err := a.svc.Attach(id); err != nil {
		return fmt.Errorf("session adapter: attach: %w", err)
	}
	return nil
}

func (a *sessionAdapter) Detach(id string) error {
	if err := a.svc.Detach(id); err != nil {
		return fmt.Errorf("session adapter: detach: %w", err)
	}
	return nil
}

func (a *sessionAdapter) Snapshot(id string) (daemon.SnapshotData, error) {
	snap, err := a.svc.Snapshot(id)
	if err != nil {
		return daemon.SnapshotData{}, fmt.Errorf("session adapter: snapshot: %w", err)
	}
	return daemon.SnapshotData{Replay: snap.Replay}, nil
}

func (a *sessionAdapter) LastActivity(id string) (time.Time, error) {
	t, err := a.svc.LastActivity(id)
	if err != nil {
		return time.Time{}, fmt.Errorf("session adapter: last activity: %w", err)
	}
	return t, nil
}

func (a *sessionAdapter) MetaSet(id, key, value string) error {
	if err := a.svc.MetaSet(id, key, value); err != nil {
		return fmt.Errorf("session adapter: meta set: %w", err)
	}
	return nil
}

func (a *sessionAdapter) MetaGet(id, key string) (string, error) {
	val, err := a.svc.MetaGet(id, key)
	if err != nil {
		return "", fmt.Errorf("session adapter: meta get: %w", err)
	}
	return val, nil
}

func (a *sessionAdapter) MetaGetAll(id string) (map[string]string, error) {
	meta, err := a.svc.MetaGetAll(id)
	if err != nil {
		return nil, fmt.Errorf("session adapter: meta get all: %w", err)
	}
	return meta, nil
}

func (a *sessionAdapter) OnExit(fn func(id string, exitCode int)) {
	a.svc.OnExit(fn)
}

func toInfo(sess session.Session) daemon.SessionInfo {
	return daemon.SessionInfo{
		ID:    sess.ID,
		State: sess.State.String(),
		Pid:   sess.Pid,
		Cols:  sess.Cols,
		Rows:  sess.Rows,
		Shell: sess.Shell,
	}
}

func (a *sessionAdapter) UpdateEmulatorScrollback(id string, scrollbackLines int) error {
	return a.svc.UpdateEmulatorScrollback(id, scrollbackLines) //nolint:wrapcheck
}

// emulatorFactoryAdapter wraps a client.EmulatorFactory to implement
// session.EmulatorFactory, bridging the public and internal types.
type emulatorFactoryAdapter struct {
	f EmulatorFactory
}

func (a *emulatorFactoryAdapter) Create(sessionID string, cols, rows int) session.ScreenEmulator {
	return &screenEmulatorAdapter{em: a.f.Create(sessionID, cols, rows)}
}

func (a *emulatorFactoryAdapter) Close() {
	a.f.Close()
}

// screenEmulatorAdapter wraps a client.ScreenEmulator to implement
// session.ScreenEmulator, converting Snapshot types.
type screenEmulatorAdapter struct {
	em ScreenEmulator
}

func (a *screenEmulatorAdapter) Process(data []byte) {
	a.em.Process(data)
}

func (a *screenEmulatorAdapter) Snapshot() session.Snapshot {
	snap := a.em.Snapshot()
	return session.Snapshot{Replay: snap.Replay}
}

func (a *screenEmulatorAdapter) Resize(cols, rows int) {
	a.em.Resize(cols, rows)
}

// SetScrollbackSize implements session.ScrollbackConfigurable by delegating to
// the underlying emulator if it supports scrollback configuration.
func (a *screenEmulatorAdapter) SetScrollbackSize(lines int) {
	type scrollbackSetter interface {
		SetScrollbackSize(lines int)
	}
	if s, ok := a.em.(scrollbackSetter); ok {
		s.SetScrollbackSize(lines)
	}
}
