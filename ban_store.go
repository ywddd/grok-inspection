package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type banStore struct {
	mu   sync.RWMutex
	bans map[string]banEntry
	rev  uint64 // monotonic revision for CAS deletes / concurrent restore

	// Serialized Save so concurrent usage/restore/unban cannot overwrite a newer snapshot.
	saveMu      sync.Mutex
	saveCond    *sync.Cond
	savePending []banEntry
	savePath    string
	saveGen     uint64
	saveWritten uint64
	saveWriting bool
}

func newBanStore() *banStore {
	s := &banStore{bans: make(map[string]banEntry)}
	s.saveCond = sync.NewCond(&s.saveMu)
	return s
}

func (s *banStore) Set(entry banEntry) {
	if entry.AuthID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.bans[entry.AuthID]; ok {
		// Keep the longer cooldown when a duplicate exhausted event races,
		// but ALWAYS advance revision so concurrent unban/restore/disable
		// cannot miss a shorter new reset event.
		if existing.ResetAt.After(entry.ResetAt) {
			entry.ResetAt = existing.ResetAt
			// Preserve window-bound semantics (manual_unban permanent bans, reason codes).
			// A shorter 429 must not rewrite a longer 403/401 manual window into quota.
			entry.ResetSource = existing.ResetSource
			if existing.ErrorCode != "" {
				// When ErrorCode is kept from the longer window, diag must match that
				// reason — never keep a new quota/401 diag under an old 403 code.
				if entry.ErrorCode != existing.ErrorCode {
					entry.ErrorCodeDiag = existing.ErrorCodeDiag
				} else if existing.ErrorCodeDiag != "" && entry.ErrorCodeDiag == "" {
					entry.ErrorCodeDiag = existing.ErrorCodeDiag
				}
				entry.ErrorCode = existing.ErrorCode
			}
		}
	}
	s.rev++
	entry.Revision = s.rev
	s.bans[entry.AuthID] = entry
}

func (s *banStore) Get(authID string) (banEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.bans[authID]
	return entry, ok
}

func (s *banStore) Delete(authID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bans[authID]; !ok {
		return false
	}
	delete(s.bans, authID)
	return true
}

// DeleteIf removes authID only when the stored revision still matches expected.
// Used by restore so a concurrent newer ban is not deleted by mistake.
func (s *banStore) DeleteIf(authID string, revision uint64) bool {
	deleted, _, _ := s.DeleteIfOrCurrent(authID, revision)
	return deleted
}

// DeleteIfOrCurrent atomically deletes authID when revision matches expected under
// store.mu. If a concurrent newer (or otherwise non-matching) entry exists, it is
// returned so callers can re-disable CPA instead of silently leaving it enabled.
//
//	deleted=true  -> row removed; present=false
//	deleted=false, present=false -> no row (already gone)
//	deleted=false, present=true  -> row remains; current is the live entry
func (s *banStore) DeleteIfOrCurrent(authID string, expected uint64) (deleted bool, current banEntry, present bool) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return false, banEntry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.bans[authID]
	if !ok {
		return false, banEntry{}, false
	}
	if entry.Revision == expected {
		delete(s.bans, authID)
		return true, banEntry{}, false
	}
	return false, entry, true
}

// DeleteIfResetAt is kept for tests that still pass reset time; prefers revision when set.
func (s *banStore) DeleteIfResetAt(authID string, resetAt time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.bans[authID]
	if !ok {
		return false
	}
	if !entry.ResetAt.Equal(resetAt) {
		return false
	}
	delete(s.bans, authID)
	return true
}

// UnsyncedCPA returns bans that have not been successfully applied to CPA yet.
func (s *banStore) UnsyncedCPA() []banEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]banEntry, 0)
	for _, entry := range s.bans {
		if !entry.CpaSynced {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BannedAt.Before(out[j].BannedAt)
	})
	return out
}

// UpdateCpaSynced marks whether the latest CPA disable succeeded.
func (s *banStore) UpdateCpaSynced(authID string, synced bool) {
	s.UpdateCpaSyncState(authID, synced, "")
}

// UpdateCpaSyncState sets sync flag and optional last error (cleared on success).
func (s *banStore) UpdateCpaSyncState(authID string, synced bool, syncErr string) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.bans[authID]
	if !ok {
		return
	}
	entry.CpaSynced = synced
	if synced {
		entry.CpaSyncError = ""
	} else if strings.TrimSpace(syncErr) != "" {
		entry.CpaSyncError = strings.TrimSpace(syncErr)
	}
	s.bans[authID] = entry
}

func (s *banStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bans = make(map[string]banEntry)
}

func (s *banStore) ClearExpired(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	expired := make([]string, 0)
	for authID, entry := range s.bans {
		if !entry.ResetAt.After(now) {
			expired = append(expired, authID)
			delete(s.bans, authID)
		}
	}
	return expired
}

func (s *banStore) Expired(now time.Time) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expired := make([]string, 0)
	for authID, entry := range s.bans {
		if !entry.ResetAt.After(now) {
			expired = append(expired, authID)
		}
	}
	return expired
}

// List returns active (not-yet-expired) bans.
func (s *banStore) List(now time.Time) []banEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]banEntry, 0, len(s.bans))
	for _, entry := range s.bans {
		if entry.ResetAt.After(now) {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ResetAt.Before(out[j].ResetAt)
	})
	return out
}

func (s *banStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.bans)
}

// snapshotEntries returns every ban including expired pending-restore entries.
func (s *banStore) snapshotEntries() []banEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]banEntry, 0, len(s.bans))
	for _, entry := range s.bans {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ResetAt.Before(out[j].ResetAt)
	})
	return out
}

// Save writes ban state. Concurrent saves are serialized; only the newest
// snapshot is guaranteed on disk. Blocks until this or a newer write finishes.
//
// Snapshot is taken under saveMu so generation assignment cannot attach a
// larger gen to an older out-of-lock snapshot.
func (s *banStore) Save(path string) error {
	if path == "" {
		return nil
	}

	s.saveMu.Lock()
	entries := s.snapshotEntries()
	s.saveGen++
	myGen := s.saveGen
	s.savePending = entries
	s.savePath = path

	for s.saveWritten < myGen {
		if s.saveWriting {
			s.saveCond.Wait()
			continue
		}
		s.saveWriting = true
		var writeErr error
		for s.savePending != nil {
			current := append([]banEntry(nil), s.savePending...)
			writeGen := s.saveGen
			writePath := s.savePath
			s.savePending = nil
			s.saveMu.Unlock()

			writeErr = writeBanEntries(writePath, current)

			s.saveMu.Lock()
			if writeErr != nil {
				if s.savePending == nil {
					s.savePending = current
					s.savePath = writePath
				}
				break
			}
			s.saveWritten = writeGen
		}
		s.saveWriting = false
		s.saveCond.Broadcast()
		if writeErr != nil && s.saveWritten < myGen {
			s.saveMu.Unlock()
			return writeErr
		}
	}
	s.saveMu.Unlock()
	return nil
}

func writeBanEntries(path string, entries []banEntry) error {
	if entries == nil {
		entries = []banEntry{}
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".grok-inspection-bans-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return replaceFileWithRetry(tempName, path)
}

func (s *banStore) Load(path string, now time.Time) error {
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Decode into a temporary structure first. Never clear live memory on corrupt JSON.
	var encoded []struct {
		AuthID        string    `json:"auth_id"`
		Provider      string    `json:"provider"`
		ErrorCode     string    `json:"error_code"`
		ErrorCodeDiag string    `json:"error_code_diag,omitempty"`
		BannedAt      time.Time `json:"banned_at"`
		ResetAt       time.Time `json:"reset_at"`
		ResetSource   string    `json:"reset_source"`
		TraceID       string    `json:"trace_id,omitempty"`
		CpaSynced     *bool     `json:"cpa_synced"`
		Revision      uint64    `json:"revision,omitempty"`
		CpaSyncError  string    `json:"cpa_sync_error,omitempty"`
	}
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return err
	}
	next := make(map[string]banEntry, len(encoded))
	var maxRev uint64
	for _, item := range encoded {
		if item.AuthID == "" {
			continue
		}
		entry := banEntry{
			AuthID:        item.AuthID,
			Provider:      item.Provider,
			ErrorCode:     item.ErrorCode,
			ErrorCodeDiag: item.ErrorCodeDiag,
			BannedAt:      item.BannedAt,
			ResetAt:       item.ResetAt,
			ResetSource:   item.ResetSource,
			TraceID:       item.TraceID,
			CpaSynced:     true, // legacy default
			Revision:      item.Revision,
			CpaSyncError:  item.CpaSyncError,
		}
		if item.CpaSynced != nil {
			entry.CpaSynced = *item.CpaSynced
		}
		if entry.Revision > maxRev {
			maxRev = entry.Revision
		}
		next[entry.AuthID] = entry
	}
	// Assign revisions for legacy files that lack them.
	s.mu.Lock()
	s.bans = next
	if maxRev == 0 {
		var rev uint64
		for id, entry := range s.bans {
			rev++
			entry.Revision = rev
			s.bans[id] = entry
		}
		s.rev = rev
	} else {
		s.rev = maxRev
	}
	s.mu.Unlock()
	_ = now
	return nil
}

// All returns every ban entry (including expired, waiting restore).
func (s *banStore) All() []banEntry {
	return s.snapshotEntries()
}
