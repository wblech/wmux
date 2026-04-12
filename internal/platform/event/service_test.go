package event

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBus_PublishAndReceive(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	sub := bus.Subscribe()
	defer sub.Unsubscribe()

	e := Event{
		Type:      SessionCreated,
		SessionID: "s1",
		Payload:   nil,
	}
	bus.Publish(e)

	select {
	case got := <-sub.Events():
		assert.Equal(t, SessionCreated, got.Type)
		assert.Equal(t, "s1", got.SessionID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	sub1 := bus.Subscribe()
	defer sub1.Unsubscribe()
	sub2 := bus.Subscribe()
	defer sub2.Unsubscribe()

	bus.Publish(Event{Type: SessionExited, SessionID: "s2", Payload: nil})

	for _, sub := range []*Subscription{sub1, sub2} {
		select {
		case got := <-sub.Events():
			assert.Equal(t, SessionExited, got.Type)
		case <-time.After(time.Second):
			t.Fatal("timed out")
		}
	}
}

func TestBus_FilterByType(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	sub := bus.SubscribeTypes(SessionExited)
	defer sub.Unsubscribe()

	bus.Publish(Event{Type: SessionCreated, SessionID: "s1", Payload: nil})
	bus.Publish(Event{Type: SessionExited, SessionID: "s2", Payload: nil})

	select {
	case got := <-sub.Events():
		assert.Equal(t, SessionExited, got.Type)
		assert.Equal(t, "s2", got.SessionID)
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	// Ensure no extra events.
	select {
	case e := <-sub.Events():
		t.Fatalf("unexpected event: %v", e)
	case <-time.After(50 * time.Millisecond):
		// OK
	}
}

func TestBus_Unsubscribe(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	sub := bus.Subscribe()
	sub.Unsubscribe()

	bus.Publish(Event{Type: SessionCreated, SessionID: "s1", Payload: nil})

	// After Unsubscribe the channel is closed, so receives yield zero values
	// with ok == false. No real events should be deliverable.
	select {
	case _, ok := <-sub.Events():
		assert.False(t, ok, "channel should be closed after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// OK
	}
}

func TestBus_DoubleUnsubscribe(_ *testing.T) {
	bus := NewBus()
	defer bus.Close()

	sub := bus.Subscribe()
	sub.Unsubscribe()
	sub.Unsubscribe() // should not panic
}

func TestBus_Close(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe()
	bus.Close()

	// Channel should be closed after bus close.
	_, ok := <-sub.Events()
	assert.False(t, ok)
}

func TestBus_PublishAfterClose(_ *testing.T) {
	bus := NewBus()
	bus.Close()

	// Should not panic.
	bus.Publish(Event{Type: SessionCreated, SessionID: "s1", Payload: nil})
}

func TestBus_SubscribeAfterClose(t *testing.T) {
	bus := NewBus()
	bus.Close()

	sub := bus.Subscribe()
	_, ok := <-sub.Events()
	assert.False(t, ok)
}

func TestBus_SlowSubscriberDoesNotBlock(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	sub := bus.Subscribe()
	defer sub.Unsubscribe()

	// Fill the subscription channel buffer.
	for range subscriberBufferSize {
		bus.Publish(Event{Type: SessionCreated, SessionID: "s1", Payload: nil})
	}

	// One more should not block (dropped).
	done := make(chan struct{})
	go func() {
		bus.Publish(Event{Type: SessionCreated, SessionID: "s1", Payload: nil})
		close(done)
	}()

	select {
	case <-done:
		// OK - did not block
	case <-time.After(time.Second):
		t.Fatal("publish blocked on slow subscriber")
	}

	_ = sub // keep sub alive
}

func TestSubscription_EventsReturnsChannel(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	sub := bus.Subscribe()
	defer sub.Unsubscribe()

	ch := sub.Events()
	require.NotNil(t, ch)
}

func TestBus_DoubleClose(_ *testing.T) {
	bus := NewBus()
	bus.Close()
	bus.Close() // should not panic
}

func TestBus_SubscribeTypesMultiple(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	sub := bus.SubscribeTypes(SessionCreated, SessionExited)
	defer sub.Unsubscribe()

	bus.Publish(Event{Type: SessionAttached, SessionID: "s1", Payload: nil})
	bus.Publish(Event{Type: SessionCreated, SessionID: "s2", Payload: nil})
	bus.Publish(Event{Type: SessionExited, SessionID: "s3", Payload: nil})

	got1 := <-sub.Events()
	assert.Equal(t, SessionCreated, got1.Type)

	got2 := <-sub.Events()
	assert.Equal(t, SessionExited, got2.Type)
}
