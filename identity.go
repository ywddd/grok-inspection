package main

import (
	"fmt"
	"sort"
	"strings"

	"grok-inspection/cpasdk/pluginapi"
)

func knownResultKeys(results []accountResult) map[string]struct{} {
	set := make(map[string]struct{}, len(results)*2)
	for _, item := range results {
		for _, key := range stableIdentityKeys(item.AuthIndex, item.FileID, item.FileName, item.FileSize, item.FileModUnix) {
			set[key] = struct{}{}
		}
	}
	return set
}

func stableIdentityKeys(authIndex, fileID, fileName string, fileSize, fileModUnix int64) []string {
	keys := make([]string, 0, 2)
	if v := strings.TrimSpace(authIndex); v != "" {
		// auth_index is the stable runtime credential id — preferred sole key.
		keys = append(keys, "ai:"+v)
		return keys
	}
	if v := strings.TrimSpace(fileID); v != "" {
		keys = append(keys, "id:"+v)
	}
	fn := strings.ToLower(strings.TrimSpace(fileName))
	if fn != "" {
		keys = append(keys, fmt.Sprintf("fn:%s|%d|%d", fn, fileSize, fileModUnix))
	}
	return keys
}

func entryIsKnown(known map[string]struct{}, file pluginapi.HostAuthFileEntry) bool {
	modUnix := int64(0)
	if !file.ModTime.IsZero() {
		modUnix = file.ModTime.Unix()
	}
	for _, key := range stableIdentityKeys(file.AuthIndex, file.ID, file.Name, file.Size, modUnix) {
		if _, ok := known[key]; ok {
			return true
		}
	}
	return false
}

func filterNewAuthEntries(files []pluginapi.HostAuthFileEntry, known map[string]struct{}, includeDisabled, onlyDisabled bool) []pluginapi.HostAuthFileEntry {
	targets := make([]pluginapi.HostAuthFileEntry, 0)
	for _, file := range files {
		if !shouldInspectEntry(file.Provider, file.Name, file.Type, file.Disabled, file.Status, includeDisabled, onlyDisabled) {
			continue
		}
		if entryIsKnown(known, file) {
			continue
		}
		targets = append(targets, file)
	}
	sort.Slice(targets, func(i, j int) bool {
		return strings.ToLower(targets[i].Name) < strings.ToLower(targets[j].Name)
	})
	return targets
}

func normalizeClassifications(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// classificationMatches reports whether a result class is in the requested set.
// Token "other" matches any class outside the four primary buckets.
func classificationMatches(class string, want map[string]struct{}) bool {
	if len(want) == 0 {
		return false
	}
	class = strings.TrimSpace(class)
	if _, ok := want[class]; ok {
		return true
	}
	if _, ok := want["other"]; !ok {
		return false
	}
	switch class {
	case "healthy", "permission_denied", "quota_exhausted", "spending_limit", "reauth":
		return false
	default:
		return true
	}
}

func resultIdentityMatch(a, b accountResult) bool {
	if ai := strings.TrimSpace(a.AuthIndex); ai != "" && ai == strings.TrimSpace(b.AuthIndex) {
		return true
	}
	if fn := strings.ToLower(strings.TrimSpace(a.FileName)); fn != "" && fn == strings.ToLower(strings.TrimSpace(b.FileName)) {
		return true
	}
	return false
}

func findResultIndex(results []accountResult, want accountResult) int {
	for i, item := range results {
		if resultIdentityMatch(item, want) {
			return i
		}
	}
	return -1
}

func matchAuthFile(files []pluginapi.HostAuthFileEntry, want accountResult) (pluginapi.HostAuthFileEntry, bool) {
	if ai := strings.TrimSpace(want.AuthIndex); ai != "" {
		for _, file := range files {
			if strings.TrimSpace(file.AuthIndex) == ai {
				return file, true
			}
		}
	}
	if fn := strings.ToLower(strings.TrimSpace(want.FileName)); fn != "" {
		for _, file := range files {
			if strings.ToLower(strings.TrimSpace(file.Name)) == fn {
				return file, true
			}
		}
	}
	return pluginapi.HostAuthFileEntry{}, false
}

// resolveClassifyTargets maps prior results to current Auth entries.
// Missing entries are returned separately so the UI can mark them.
func resolveClassifyTargets(files []pluginapi.HostAuthFileEntry, selected []accountResult) (targets []pluginapi.HostAuthFileEntry, missing []accountResult) {
	seen := make(map[string]struct{}, len(selected))
	for _, item := range selected {
		file, ok := matchAuthFile(files, item)
		if !ok {
			missing = append(missing, item)
			continue
		}
		key := strings.TrimSpace(file.AuthIndex)
		if key == "" {
			key = "name:" + strings.ToLower(strings.TrimSpace(file.Name))
		} else {
			key = "ai:" + key
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, file)
	}
	sort.Slice(targets, func(i, j int) bool {
		return strings.ToLower(targets[i].Name) < strings.ToLower(targets[j].Name)
	})
	return targets, missing
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

// cancelledAccountResult marks an account that was not probed because the job stopped.
func cancelledAccountResult(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
	name := firstNonEmpty(file.Email, file.Label, file.Name, file.AuthIndex, file.ID)
	base := accountResult{
		AuthIndex:      file.AuthIndex,
		Name:           name,
		FileName:       file.Name,
		Email:          file.Email,
		FileID:         file.ID,
		FileSize:       file.Size,
		Disabled:       file.Disabled || isDisabledEntry(file.Disabled, file.Status),
		Model:          model,
		Classification: "probe_error",
		Action:         "keep",
		Reason:         T(normalizeLang(string(lang)), "stopped_before_probe"),
	}
	if !file.ModTime.IsZero() {
		base.FileModUnix = file.ModTime.Unix()
	}
	return base
}

func resultContainsAuthFile(results []accountResult, file pluginapi.HostAuthFileEntry) bool {
	modUnix := int64(0)
	if !file.ModTime.IsZero() {
		modUnix = file.ModTime.Unix()
	}
	wantKeys := stableIdentityKeys(file.AuthIndex, file.ID, file.Name, file.Size, modUnix)
	if len(wantKeys) == 0 {
		// Fall back to loose name match when no stable key exists.
		name := firstNonEmpty(file.Email, file.Label, file.Name, file.AuthIndex, file.ID)
		for _, item := range results {
			if item.AuthIndex != "" && item.AuthIndex == file.AuthIndex {
				return true
			}
			if name != "" && (item.Name == name || item.Email == name || item.FileName == file.Name) {
				return true
			}
		}
		return false
	}
	want := make(map[string]struct{}, len(wantKeys))
	for _, k := range wantKeys {
		want[k] = struct{}{}
	}
	for _, item := range results {
		for _, k := range stableIdentityKeys(item.AuthIndex, item.FileID, item.FileName, item.FileSize, item.FileModUnix) {
			if _, ok := want[k]; ok {
				return true
			}
		}
	}
	return false
}
