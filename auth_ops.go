package main

import (
	"strings"
	"sync"
)

// Per-account serialization for network mutation + local ban commit paths.
// Prevents disable/restore/unban races from diverging CPA vs local ban pool.
var authOpLocks sync.Map // map[string]*sync.Mutex

func withAuthOp(authID string, fn func() error) error {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return fn()
	}
	v, _ := authOpLocks.LoadOrStore(authID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return fn()
}
