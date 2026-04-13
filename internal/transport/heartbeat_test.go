package transport

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/ipc"
	"github.com/wblech/wmux/internal/platform/protocol"
)

func TestHeartbeatManager_SendsHeartbeat(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	streamConn := protocol.NewConn(server)
	clientConn := protocol.NewConn(client)

	now := time.Now()
	c := &Client{
		ID:               "hb-client",
		Control:          nil,
		Stream:           streamConn,
		Creds:            ipc.PeerCredentials{UID: 0, PID: 0},
		ConnectedAt:      now,
		LastHeartbeatAck: now,
		MissedHeartbeats: 0,
	}

	hm := NewHeartbeatManager(50*time.Millisecond, 3)
	hm.Track(c)

	stop := hm.Start()
	defer stop()

	f, err := clientConn.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgHeartbeat, f.Type)
}

func TestHeartbeatManager_AckResetsCounter(t *testing.T) {
	hm := NewHeartbeatManager(50*time.Millisecond, 3)

	now := time.Now()
	c := &Client{
		ID:               "ack-client",
		Control:          nil,
		Stream:           nil,
		Creds:            ipc.PeerCredentials{UID: 0, PID: 0},
		ConnectedAt:      now,
		LastHeartbeatAck: now,
		MissedHeartbeats: 2,
	}

	hm.Track(c)
	hm.Ack("ack-client")

	hm.mu.RLock()
	tracked := hm.clients["ack-client"]
	hm.mu.RUnlock()

	assert.Equal(t, 0, tracked.MissedHeartbeats)
}

func TestHeartbeatManager_AckUnknownClient(_ *testing.T) {
	hm := NewHeartbeatManager(50*time.Millisecond, 3)
	// Should not panic.
	hm.Ack("nonexistent")
}

func TestHeartbeatManager_DeadClientCallback(t *testing.T) {
	server, client := net.Pipe()
	_ = client.Close() // Close client side so writes fail.

	streamConn := protocol.NewConn(server)
	now := time.Now()
	c := &Client{
		ID:               "dead-client",
		Control:          nil,
		Stream:           streamConn,
		Creds:            ipc.PeerCredentials{UID: 0, PID: 0},
		ConnectedAt:      now,
		LastHeartbeatAck: now,
		MissedHeartbeats: 0,
	}

	deadCh := make(chan string, 1)
	hm := NewHeartbeatManager(20*time.Millisecond, 2)
	hm.OnDead(func(clientID string) {
		deadCh <- clientID
	})
	hm.Track(c)

	stop := hm.Start()
	defer stop()

	select {
	case id := <-deadCh:
		assert.Equal(t, "dead-client", id)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for dead client callback")
	}

	_ = server.Close()
}

func TestHeartbeatManager_Untrack(t *testing.T) {
	hm := NewHeartbeatManager(50*time.Millisecond, 3)

	now := time.Now()
	c := &Client{
		ID:               "untrack-me",
		Control:          nil,
		Stream:           nil,
		Creds:            ipc.PeerCredentials{UID: 0, PID: 0},
		ConnectedAt:      now,
		LastHeartbeatAck: now,
		MissedHeartbeats: 0,
	}

	hm.Track(c)
	hm.Untrack("untrack-me")

	hm.mu.RLock()
	_, exists := hm.clients["untrack-me"]
	hm.mu.RUnlock()

	assert.False(t, exists)
}

func TestHeartbeatManager_Stop(_ *testing.T) {
	hm := NewHeartbeatManager(50*time.Millisecond, 3)
	stop := hm.Start()
	stop()
	// Calling stop again should not panic.
	stop()
}

func TestHeartbeatManager_SkipsNilStream(t *testing.T) {
	hm := NewHeartbeatManager(50*time.Millisecond, 3)

	now := time.Now()
	c := &Client{
		ID:               "no-stream",
		Control:          nil,
		Stream:           nil,
		Creds:            ipc.PeerCredentials{UID: 0, PID: 0},
		ConnectedAt:      now,
		LastHeartbeatAck: now,
		MissedHeartbeats: 0,
	}

	hm.Track(c)

	// Manually tick — should not panic or error on nil stream.
	hm.tick()

	hm.mu.RLock()
	_, exists := hm.clients["no-stream"]
	hm.mu.RUnlock()
	assert.True(t, exists)
}

func TestHeartbeatManager_ExceedsMissedWithoutWrite(t *testing.T) {
	// Client with stream=nil and high missed count should get reaped
	// once it has a stream that fails.
	server, client := net.Pipe()
	_ = client.Close()

	streamConn := protocol.NewConn(server)
	now := time.Now()
	c := &Client{
		ID:               "exceed-missed",
		Control:          nil,
		Stream:           streamConn,
		Creds:            ipc.PeerCredentials{UID: 0, PID: 0},
		ConnectedAt:      now,
		LastHeartbeatAck: now,
		MissedHeartbeats: 5, // already over maxMissed of 3
	}

	deadCh := make(chan string, 1)
	hm := NewHeartbeatManager(20*time.Millisecond, 3)
	hm.OnDead(func(clientID string) {
		deadCh <- clientID
	})
	hm.Track(c)

	// One tick should detect it as dead.
	hm.tick()

	select {
	case id := <-deadCh:
		assert.Equal(t, "exceed-missed", id)
	default:
		t.Fatal("expected dead callback")
	}

	_ = server.Close()
}
