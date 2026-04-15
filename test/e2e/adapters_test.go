package e2e

import (
	"context"
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
	return a.srv.Serve(ctx) //nolint:wrapcheck
}

func (a *serverAdapter) BroadcastTo(clientID string, f protocol.Frame) error {
	return a.srv.BroadcastTo(clientID, f) //nolint:wrapcheck
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
		return daemon.SessionInfo{}, err //nolint:wrapcheck
	}

	return daemon.SessionInfo{
		ID:    sess.ID,
		State: sess.State.String(),
		Pid:   sess.Pid,
		Cols:  sess.Cols,
		Rows:  sess.Rows,
		Shell: sess.Shell,
	}, nil
}

func (a *sessionAdapter) Get(id string) (daemon.SessionInfo, error) {
	sess, err := a.svc.Get(id)
	if err != nil {
		return daemon.SessionInfo{}, err //nolint:wrapcheck
	}

	return daemon.SessionInfo{
		ID:    sess.ID,
		State: sess.State.String(),
		Pid:   sess.Pid,
		Cols:  sess.Cols,
		Rows:  sess.Rows,
		Shell: sess.Shell,
	}, nil
}

func (a *sessionAdapter) List() []daemon.SessionInfo {
	sessions := a.svc.List()
	infos := make([]daemon.SessionInfo, 0, len(sessions))

	for _, sess := range sessions {
		infos = append(infos, daemon.SessionInfo{
			ID:    sess.ID,
			State: sess.State.String(),
			Pid:   sess.Pid,
			Cols:  sess.Cols,
			Rows:  sess.Rows,
			Shell: sess.Shell,
		})
	}

	return infos
}

func (a *sessionAdapter) Kill(id string) error {
	return a.svc.Kill(id) //nolint:wrapcheck
}

func (a *sessionAdapter) Resize(id string, cols, rows int) error {
	return a.svc.Resize(id, cols, rows) //nolint:wrapcheck
}

func (a *sessionAdapter) WriteInput(id string, data []byte) error {
	return a.svc.WriteInput(id, data) //nolint:wrapcheck
}

func (a *sessionAdapter) ReadOutput(id string) ([]byte, error) {
	return a.svc.ReadOutput(id) //nolint:wrapcheck
}

func (a *sessionAdapter) Attach(id string) error {
	return a.svc.Attach(id) //nolint:wrapcheck
}

func (a *sessionAdapter) Detach(id string) error {
	return a.svc.Detach(id) //nolint:wrapcheck
}

func (a *sessionAdapter) Snapshot(id string) (daemon.SnapshotData, error) {
	snap, err := a.svc.Snapshot(id)
	if err != nil {
		return daemon.SnapshotData{}, err //nolint:wrapcheck
	}

	return daemon.SnapshotData{Scrollback: snap.Scrollback, Viewport: snap.Viewport}, nil
}

func (a *sessionAdapter) LastActivity(id string) (time.Time, error) {
	return a.svc.LastActivity(id) //nolint:wrapcheck
}

func (a *sessionAdapter) MetaSet(id, key, value string) error {
	return a.svc.MetaSet(id, key, value) //nolint:wrapcheck
}

func (a *sessionAdapter) MetaGet(id, key string) (string, error) {
	return a.svc.MetaGet(id, key) //nolint:wrapcheck
}

func (a *sessionAdapter) MetaGetAll(id string) (map[string]string, error) {
	return a.svc.MetaGetAll(id) //nolint:wrapcheck
}

func (a *sessionAdapter) OnExit(fn func(id string, exitCode int)) {
	a.svc.OnExit(fn)
}
