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

// NewEmulatorFactory returns an emulator factory for direct, in-process use
// by consumers that need a ScreenEmulator without wiring a full daemon (for
// example, a downstream frontend applying a Replay snapshot).
func NewEmulatorFactory(opts ...Option) client.EmulatorFactory {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}
	return &factory{cfg: cfg}
}
