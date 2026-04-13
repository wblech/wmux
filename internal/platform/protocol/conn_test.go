package protocol

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConn_RoundTrip(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	t.Cleanup(func() { _ = client.Close() })

	sc := NewConn(server)
	cc := NewConn(client)

	want := Frame{
		Version: ProtocolVersion,
		Type:    MsgData,
		Payload: []byte("hello"),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		err := cc.WriteFrame(want)
		assert.NoError(t, err)
	}()

	got, err := sc.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, want.Version, got.Version)
	assert.Equal(t, want.Type, got.Type)
	assert.Equal(t, want.Payload, got.Payload)

	<-done
}

func TestConn_ReadFrame_Closed(t *testing.T) {
	server, client := net.Pipe()
	_ = client.Close()

	sc := NewConn(server)
	_, err := sc.ReadFrame()
	require.Error(t, err)
	_ = server.Close()
}

func TestConn_Close(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })

	cc := NewConn(client)
	err := cc.Close()
	assert.NoError(t, err)
}

func TestConn_Raw(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	t.Cleanup(func() { _ = client.Close() })

	cc := NewConn(client)
	assert.Equal(t, client, cc.Raw())
}

func TestConn_MultipleFrames(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	t.Cleanup(func() { _ = client.Close() })

	sc := NewConn(server)
	cc := NewConn(client)

	frames := []Frame{
		{Version: ProtocolVersion, Type: MsgCreate, Payload: []byte("s1")},
		{Version: ProtocolVersion, Type: MsgOK, Payload: nil},
		{Version: ProtocolVersion, Type: MsgData, Payload: []byte("output bytes")},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, f := range frames {
			err := cc.WriteFrame(f)
			assert.NoError(t, err)
		}
	}()

	for _, want := range frames {
		got, err := sc.ReadFrame()
		require.NoError(t, err)
		assert.Equal(t, want.Type, got.Type)
		assert.Equal(t, want.Payload, got.Payload)
	}

	<-done
}
