package eventbus

import (
	"sync/atomic"
	"testing"
)

func TestEmitAndOn(t *testing.T) {
	bus := New()
	var received atomic.Int32

	bus.On("test", func(data any) {
		if data.(string) == "hello" {
			received.Add(1)
		}
	})

	bus.Emit("test", "hello")
	bus.Emit("test", "hello")

	if received.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", received.Load())
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := New()
	var received atomic.Int32

	unsub := bus.On("test", func(data any) {
		received.Add(1)
	})

	bus.Emit("test", nil)
	unsub()
	bus.Emit("test", nil)

	if received.Load() != 1 {
		t.Errorf("expected 1 call after unsub, got %d", received.Load())
	}
}

func TestMultipleChannels(t *testing.T) {
	bus := New()
	var a, b atomic.Int32

	bus.On("a", func(data any) { a.Add(1) })
	bus.On("b", func(data any) { b.Add(1) })

	bus.Emit("a", nil)
	bus.Emit("b", nil)
	bus.Emit("a", nil)

	if a.Load() != 2 {
		t.Errorf("channel a: expected 2, got %d", a.Load())
	}
	if b.Load() != 1 {
		t.Errorf("channel b: expected 1, got %d", b.Load())
	}
}

func TestClear(t *testing.T) {
	bus := New()
	var received atomic.Int32

	bus.On("test", func(data any) { received.Add(1) })
	bus.Emit("test", nil)
	bus.Clear()
	bus.Emit("test", nil)

	if received.Load() != 1 {
		t.Errorf("expected 1 after clear, got %d", received.Load())
	}
}

func TestHandlerPanicRecovery(t *testing.T) {
	bus := New()
	var received atomic.Int32

	bus.On("test", func(data any) { panic("boom") })
	bus.On("test", func(data any) { received.Add(1) })

	bus.Emit("test", nil)

	// Second handler should still fire
	if received.Load() != 1 {
		t.Errorf("expected 1 after panic recovery, got %d", received.Load())
	}
}
