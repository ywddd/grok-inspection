package main

import (
	"fmt"
	"grok-inspection/cpasdk/pluginapi"
	"strings"
)

func hostAuthFromAccountResult(item *accountResult) *pluginapi.HostAuthFileEntry {
	if item == nil {
		return nil
	}
	fileName := firstNonEmpty(item.FileName, item.Name)
	if strings.TrimSpace(fileName) == "" {
		return nil
	}
	return &pluginapi.HostAuthFileEntry{
		AuthIndex: item.AuthIndex,
		Name:      fileName,
		ID:        firstNonEmpty(item.FileName, item.AuthIndex),
		Email:     item.Email,
		Disabled:  item.Disabled,
	}
}

// findAuthFromResults resolves a Management/CPA target string with prioritized scans.
// UI and bulk paths pass physical file names, so FileName/FileID wins over AuthIndex:
//  1. physical FileName / FileID
//  2. strict AuthIndex
//  3. display Name / Email
func findAuthFromResults(name string) *pluginapi.HostAuthFileEntry {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()

	for i := range engine.results {
		item := &engine.results[i]
		if strings.TrimSpace(item.FileName) == name || strings.TrimSpace(item.FileID) == name {
			return hostAuthFromAccountResult(item)
		}
	}
	for i := range engine.results {
		item := &engine.results[i]
		if strings.TrimSpace(item.AuthIndex) == name {
			return hostAuthFromAccountResult(item)
		}
	}
	for i := range engine.results {
		item := &engine.results[i]
		if strings.TrimSpace(item.Name) == name || strings.TrimSpace(item.Email) == name {
			return hostAuthFromAccountResult(item)
		}
	}
	return nil
}

// findAuthInHostList mirrors Management file-name semantics on the host auth list:
//  1. physical Name / ID
//  2. AuthIndex
//  3. display Email / Label
func findAuthInHostList(files []pluginapi.HostAuthFileEntry, name string) *pluginapi.HostAuthFileEntry {
	name = strings.TrimSpace(name)
	if name == "" || len(files) == 0 {
		return nil
	}
	for i := range files {
		file := &files[i]
		if strings.TrimSpace(file.Name) == name || strings.TrimSpace(file.ID) == name {
			return file
		}
	}
	for i := range files {
		file := &files[i]
		if strings.TrimSpace(file.AuthIndex) == name {
			return file
		}
	}
	for i := range files {
		file := &files[i]
		if strings.TrimSpace(file.Email) == name || strings.TrimSpace(file.Label) == name {
			return file
		}
	}
	return nil
}

func findAuthFile(name string) (*pluginapi.HostAuthFileEntry, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	// Fast path: after a full/incremental inspect we already know file names.
	// Avoid listing 1000+ CPA auth files on every enable/disable/delete click.
	if entry := findAuthFromResults(name); entry != nil {
		return entry, nil
	}
	list, errList := callHostAuthListFn()
	if errList != nil {
		return nil, errList
	}
	if entry := findAuthInHostList(list.Files, name); entry != nil {
		return entry, nil
	}
	return nil, fmt.Errorf("auth not found: %s", name)
}

// resolveAccountResultByTargetID maps one free-form target id to at most one
// inspection row with deterministic priority:
//  1. AuthIndex
//  2. FileName / FileID
//  3. display Name
//  4. Email
//
// This prevents a scheduled AuthIndex from also selecting another row whose
// FileName/Name/Email collides with that same token (itemSelected multi-match).
func resolveAccountResultByTargetID(results []accountResult, id string) (accountResult, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return accountResult{}, false
	}
	for _, item := range results {
		if strings.TrimSpace(item.AuthIndex) == id {
			return item, true
		}
	}
	for _, item := range results {
		if strings.TrimSpace(item.FileName) == id || strings.TrimSpace(item.FileID) == id {
			return item, true
		}
	}
	for _, item := range results {
		if strings.TrimSpace(item.Name) == id {
			return item, true
		}
	}
	for _, item := range results {
		if strings.TrimSpace(item.Email) == id {
			return item, true
		}
	}
	return accountResult{}, false
}

// collectAccountsByTargetIDs resolves each target id to at most one row (priority
// above), preserving request order and de-duplicating identical row identities.
func collectAccountsByTargetIDs(results []accountResult, ids []string) []accountResult {
	out := make([]accountResult, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		item, ok := resolveAccountResultByTargetID(results, id)
		if !ok {
			continue
		}
		key := firstNonEmpty(item.AuthIndex, item.FileName, item.FileID, item.Name, item.Email)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

// lookupAccountResultByFileFirst resolves a free-form name/token for inspection-row
// fallback: physical file keys first, then AuthIndex, then display fields.
func lookupAccountResultByFileFirst(results []accountResult, id string) (accountResult, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return accountResult{}, false
	}
	for _, item := range results {
		if strings.TrimSpace(item.FileName) == id || strings.TrimSpace(item.FileID) == id {
			return item, true
		}
	}
	for _, item := range results {
		if strings.TrimSpace(item.AuthIndex) == id {
			return item, true
		}
	}
	for _, item := range results {
		if strings.TrimSpace(item.Name) == id || strings.TrimSpace(item.Email) == id {
			return item, true
		}
	}
	return accountResult{}, false
}

func resultMatchesTarget(item accountResult, target *pluginapi.HostAuthFileEntry, name string) bool {
	name = strings.TrimSpace(name)
	if target != nil {
		if item.AuthIndex != "" && item.AuthIndex == target.AuthIndex {
			return true
		}
		if item.FileName != "" && (item.FileName == target.Name || item.FileName == target.ID) {
			return true
		}
		if item.FileID != "" && (item.FileID == target.Name || item.FileID == target.ID) {
			return true
		}
		if item.Name != "" && (item.Name == target.Name || item.Name == target.Email || item.Name == target.ID) {
			return true
		}
	}
	if name == "" {
		return false
	}
	return item.AuthIndex == name ||
		item.FileName == name ||
		item.FileID == name ||
		item.Name == name ||
		item.Email == name
}

func banAliases(target *pluginapi.HostAuthFileEntry, name string) map[string]struct{} {
	aliases := map[string]struct{}{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" {
			aliases[v] = struct{}{}
		}
	}
	add(name)
	if target != nil {
		add(target.AuthIndex)
		add(target.Name)
		add(target.ID)
		add(target.Email)
		add(target.Label)
	}
	return aliases
}

func banIDMatchesAliases(authID string, aliases map[string]struct{}) bool {
	id := strings.TrimSpace(authID)
	if id == "" {
		return false
	}
	if _, ok := aliases[id]; ok {
		return true
	}
	base := id
	if i := strings.LastIndexAny(id, `/\`); i >= 0 {
		base = id[i+1:]
	}
	_, ok := aliases[base]
	return ok
}
