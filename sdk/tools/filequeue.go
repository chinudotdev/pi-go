package tools

import (
	"os"
	"path/filepath"
	"sync"
)

// global mutation queues: maps resolved file path -> mutex
var (
	mutationQueues   = make(map[string]*mutationEntry)
	mutationQueuesMu sync.Mutex
)

type mutationEntry struct {
	mu    sync.Mutex
	count int
}

// WithFileMutationQueue serializes file mutation operations targeting the same file.
// Operations for different files still run in parallel.
func WithFileMutationQueue(filePath string, fn func() error) error {
	key := resolveMutationKey(filePath)

	mutationQueuesMu.Lock()
	entry, ok := mutationQueues[key]
	if !ok {
		entry = &mutationEntry{}
		mutationQueues[key] = entry
	}
	entry.count++
	mutationQueuesMu.Unlock()

	entry.mu.Lock()
	defer func() {
		entry.mu.Unlock()
		mutationQueuesMu.Lock()
		entry.count--
		if entry.count <= 0 {
			delete(mutationQueues, key)
		}
		mutationQueuesMu.Unlock()
	}()

	return fn()
}

func resolveMutationKey(filePath string) string {
	resolved := filepath.Clean(filePath)
	real, err := filepath.EvalSymlinks(resolved)
	if err == nil {
		return real
	}
	if isMissingPathError(err) {
		return resolved
	}
	return resolved
}

func isMissingPathError(err error) bool {
	if pe, ok := err.(*os.PathError); ok {
		return pe.Err == os.ErrNotExist || pe.Err == os.ErrInvalid
	}
	return os.IsNotExist(err) || os.IsPermission(err)
}
