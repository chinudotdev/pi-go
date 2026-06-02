package ai

import (
	"reflect"
	"sync"
)

// SessionResourceCleanup is a function that cleans up session resources.
type SessionResourceCleanup func(sessionID string)

var (
	sessionCleanupsMu sync.Mutex
	sessionCleanups   []SessionResourceCleanup
)

// RegisterSessionResourceCleanup registers a cleanup function.
// Returns a function to unregister it.
func RegisterSessionResourceCleanup(cleanup SessionResourceCleanup) func() {
	sessionCleanupsMu.Lock()
	defer sessionCleanupsMu.Unlock()

	sessionCleanups = append(sessionCleanups, cleanup)
	return func() {
		sessionCleanupsMu.Lock()
		defer sessionCleanupsMu.Unlock()
		cleanupPtr := reflect.ValueOf(cleanup).Pointer()
		for i, c := range sessionCleanups {
			if reflect.ValueOf(c).Pointer() == cleanupPtr {
				sessionCleanups = append(sessionCleanups[:i], sessionCleanups[i+1:]...)
				break
			}
		}
	}
}

// CleanupSessionResources runs all registered cleanup functions.
func CleanupSessionResources(sessionID string) error {
	sessionCleanupsMu.Lock()
	cleanups := make([]SessionResourceCleanup, len(sessionCleanups))
	copy(cleanups, sessionCleanups)
	sessionCleanupsMu.Unlock()

	var errs []error
	for _, cleanup := range cleanups {
		if err := func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					// catch panics from cleanup functions
				}
			}()
			cleanup(sessionID)
			return nil
		}(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return &SessionCleanupError{Errors: errs}
	}
	return nil
}

// SessionCleanupError represents multiple cleanup failures.
type SessionCleanupError struct {
	Errors []error
}

func (e *SessionCleanupError) Error() string {
	return "failed to cleanup session resources"
}

func (e *SessionCleanupError) Unwrap() []error {
	return e.Errors
}
