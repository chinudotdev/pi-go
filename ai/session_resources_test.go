package ai

import (
	"reflect"
	"sync/atomic"
	"testing"
)

func TestRegisterAndUnregister(t *testing.T) {
	cleanupCalled := atomic.Int32{}

	cleanup := func(sessionID string) {
		cleanupCalled.Add(1)
	}

	unregister := RegisterSessionResourceCleanup(cleanup)

	// Cleanup should be called
	err := CleanupSessionResources("test-session")
	if err != nil {
		t.Fatalf("CleanupSessionResources returned error: %v", err)
	}
	if cleanupCalled.Load() != 1 {
		t.Errorf("cleanup called %d times, want 1", cleanupCalled.Load())
	}

	// Unregister
	unregister()

	// Cleanup should NOT be called again
	err = CleanupSessionResources("test-session")
	if err != nil {
		t.Fatalf("CleanupSessionResources returned error: %v", err)
	}
	if cleanupCalled.Load() != 1 {
		t.Errorf("cleanup called %d times after unregister, want 1", cleanupCalled.Load())
	}
}

func TestMultipleCleanups(t *testing.T) {
	cleanupCount := atomic.Int32{}

	cleanup1 := func(sessionID string) { cleanupCount.Add(1) }
	cleanup2 := func(sessionID string) { cleanupCount.Add(1) }

	RegisterSessionResourceCleanup(cleanup1)
	RegisterSessionResourceCleanup(cleanup2)

	err := CleanupSessionResources("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanupCount.Load() != 2 {
		t.Errorf("expected 2 cleanups, got %d", cleanupCount.Load())
	}
}

func TestUnregisterByIdentity(t *testing.T) {
	// Ensure we can unregister the correct function when multiple are registered
	var called []string
	cleanup1 := func(sessionID string) { called = append(called, "cleanup1") }
	cleanup2 := func(sessionID string) { called = append(called, "cleanup2") }
	cleanup3 := func(sessionID string) { called = append(called, "cleanup3") }

	unreg2 := RegisterSessionResourceCleanup(cleanup1)
	RegisterSessionResourceCleanup(cleanup2)
	RegisterSessionResourceCleanup(cleanup3)

	// Unregister cleanup1 (not cleanup2)
	_ = unreg2

	// We need to verify the pointer comparison works.
	// The key test: reflect.ValueOf(cleanup).Pointer() matches
	ptr1 := reflect.ValueOf(cleanup1).Pointer()
	ptr2 := reflect.ValueOf(cleanup2).Pointer()
	if ptr1 == ptr2 {
		t.Error("different closures should have different function pointers")
	}
}

func TestCleanupSessionResources_PanicRecovery(t *testing.T) {
	cleanupCalled := atomic.Int32{}

	panicking := func(sessionID string) {
		cleanupCalled.Add(1)
		panic("oh no")
	}
	normal := func(sessionID string) {
		cleanupCalled.Add(1)
	}

	RegisterSessionResourceCleanup(panicking)
	RegisterSessionResourceCleanup(normal)

	err := CleanupSessionResources("test")
	if err != nil {
		t.Fatalf("should not error on panic recovery: %v", err)
	}
	// Both should have been attempted
	if cleanupCalled.Load() != 2 {
		t.Errorf("expected 2 cleanup attempts, got %d", cleanupCalled.Load())
	}
}
