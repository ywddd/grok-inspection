package main

import (
	"fmt"
	"grok-inspection/cpasdk/pluginapi"
	"strings"
)

func findAuthFromResults(name string) *pluginapi.HostAuthFileEntry {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	for i := range engine.results {
		item := &engine.results[i]
		if item.AuthIndex == name || item.FileName == name || item.Name == name || item.Email == name {
			fileName := firstNonEmpty(item.FileName, item.Name)
			if fileName == "" {
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
	for i := range list.Files {
		file := &list.Files[i]
		if file.Name == name || file.ID == name || file.AuthIndex == name || file.Email == name {
			return file, nil
		}
	}
	return nil, fmt.Errorf("auth not found: %s", name)
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
		if item.Name != "" && (item.Name == target.Name || item.Name == target.Email || item.Name == target.ID) {
			return true
		}
	}
	if name == "" {
		return false
	}
	return item.AuthIndex == name || item.FileName == name || item.Name == name || item.Email == name
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
