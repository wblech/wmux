package charmvt

import "github.com/wblech/wmux/pkg/client"

type factory struct {
	cfg *config
}

func (f *factory) Create(sessionID string, cols, rows int) client.ScreenEmulator {
	return newEmulator(sessionID, cols, rows, f.cfg)
}

func (f *factory) Close() {}

// Backend returns a client.Option that configures the daemon to use
// charmbracelet/x/vt as the in-process terminal emulator.
func Backend(opts ...Option) client.Option {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}
	return client.WithEmulatorFactory(&factory{cfg: cfg})
}
