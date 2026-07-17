package main

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
)

const (
	defaultBaseURLWorkers = 6
	maxBaseURLWorkers     = 8
)

type baseURLApplyRequest struct {
	// AuthIndexes limits the set; empty means all results with classification api_gateway_ok
	// (or preferred_base_url already pointing at official API but missing using_api).
	AuthIndexes []string `json:"auth_indexes"`
	Workers     int      `json:"workers"`
	// TargetBaseURL defaults to https://api.x.ai/v1
	TargetBaseURL string `json:"target_base_url"`
	// SkipReprobe skips the post-write api.x.ai re-check.
	SkipReprobe bool `json:"skip_reprobe"`
}

func (e *inspectionEngine) startBaseURLApply(req baseURLApplyRequest) error {
	workers := req.Workers
	if workers == 0 {
		workers = defaultBaseURLWorkers
	}
	if workers < 1 || workers > maxBaseURLWorkers {
		return fmt.Errorf("workers must be between 1 and %d", maxBaseURLWorkers)
	}
	target := strings.TrimSpace(req.TargetBaseURL)
	if target == "" {
		target = xaiOfficialAPIBaseURL
	}
	target = strings.TrimRight(target, "/")

	e.mu.Lock()
	if e.running || e.applying || e.reauthing || e.baseURLApplying {
		e.mu.Unlock()
		return fmt.Errorf("inspection already running")
	}
	if e.actionInFlight > 0 {
		e.mu.Unlock()
		return fmt.Errorf("busy: row action in progress")
	}
	results := append([]accountResult(nil), e.results...)
	e.mu.Unlock()

	targets := selectBaseURLSwitchTargets(results, req.AuthIndexes)
	if len(targets) == 0 {
		return fmt.Errorf("没有可切换 base_url 的账号（需要巡检结果为 api_gateway_ok：CLI 拒绝但 api.x.ai 可用）")
	}

	e.mu.Lock()
	if e.running || e.applying || e.reauthing || e.baseURLApplying {
		e.mu.Unlock()
		return fmt.Errorf("inspection already running")
	}
	e.baseURLApplying = true
	e.baseURLStop = false
	e.baseURLDone = 0
	e.baseURLTotal = len(targets)
	e.baseURLSuccesses = 0
	e.baseURLCurrent = ""
	e.baseURLFailures = nil
	e.mu.Unlock()

	e.runWG.Add(1)
	go func() {
		defer e.runWG.Done()
		defer func() {
			e.mu.Lock()
			e.baseURLApplying = false
			e.baseURLCurrent = ""
			e.persistLocked()
			e.mu.Unlock()
		}()
		e.runBaseURLApply(targets, workers, target, req.SkipReprobe)
	}()
	return nil
}

func (e *inspectionEngine) stopBaseURLApply() {
	e.mu.Lock()
	e.baseURLStop = true
	e.mu.Unlock()
}

func selectBaseURLSwitchTargets(results []accountResult, authIndexes []string) []accountResult {
	want := map[string]struct{}{}
	for _, key := range authIndexes {
		key = strings.TrimSpace(key)
		if key != "" {
			want[key] = struct{}{}
		}
	}
	out := make([]accountResult, 0)
	for _, item := range results {
		if len(want) > 0 {
			if _, ok := want[item.AuthIndex]; !ok {
				if _, ok2 := want[item.FileName]; !ok2 {
					if _, ok3 := want[item.Name]; !ok3 {
						continue
					}
				}
			}
		} else if !isBaseURLSwitchEligible(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func isBaseURLSwitchEligible(item accountResult) bool {
	if strings.TrimSpace(item.Classification) == "api_gateway_ok" {
		return true
	}
	// Already preferred official API from dual probe but not yet applied.
	if strings.TrimSpace(item.PreferredBaseURL) == xaiOfficialAPIBaseURL &&
		(item.APIHTTPStatus >= 200 && item.APIHTTPStatus < 300) &&
		!item.UsingAPI {
		return true
	}
	return false
}

func (e *inspectionEngine) runBaseURLApply(targets []accountResult, workers int, target string, skipReprobe bool) {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, item := range targets {
		e.mu.Lock()
		stop := e.baseURLStop
		e.mu.Unlock()
		if stop {
			break
		}
		item := item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			label := firstNonEmpty(item.Email, item.Name, item.AuthIndex)
			e.mu.Lock()
			e.baseURLCurrent = label
			e.mu.Unlock()

			err := applyOfficialAPIBaseURL(item, target, skipReprobe)
			e.mu.Lock()
			e.baseURLDone++
			if err != nil {
				e.baseURLFailures = append(e.baseURLFailures, label+": "+err.Error())
			} else {
				e.baseURLSuccesses++
			}
			e.mu.Unlock()
		}()
	}
	wg.Wait()
}

func applyOfficialAPIBaseURL(item accountResult, target string, skipReprobe bool) error {
	authIndex := strings.TrimSpace(item.AuthIndex)
	if authIndex == "" {
		return fmt.Errorf("缺少 auth_index")
	}
	fileName, data, errGet := loadAuthJSONMap(authIndex)
	if errGet != nil {
		return errGet
	}
	if fileName == "" {
		fileName = firstNonEmpty(item.FileName, item.Name)
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".json") {
		// host.auth.save expects a .json basename
		base := path.Base(fileName)
		if strings.HasSuffix(strings.ToLower(base), ".json") {
			fileName = base
		} else {
			return fmt.Errorf("auth 文件名无效: %s", fileName)
		}
	}

	// Write fields CPA xai_executor honors for official API chat:
	// using_api=true + base_url=https://api.x.ai/v1 (token unchanged — no re-login).
	data["base_url"] = target
	data["using_api"] = true
	if asString(data["type"]) == "" {
		data["type"] = "xai"
	}
	if asString(data["auth_kind"]) == "" {
		data["auth_kind"] = "oauth"
	}
	// Keep refresh/token fields; do not invent CLI-only headers when switching to API.
	// If headers only contain CLI identity keys, leave them (CPA strips for non-CLI host).

	raw, errMarshal := json.Marshal(data)
	if errMarshal != nil {
		return errMarshal
	}
	if errSave := callHostAuthSave(fileName, raw); errSave != nil {
		return errSave
	}

	// Optional: confirm api.x.ai still works with the same token.
	if !skipReprobe {
		token := firstNonEmpty(asString(data["access_token"]), asString(data["token"]), asString(data["api_key"]))
		if token != "" {
			body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"max_tokens":8,"stream":false}`, defaultProbeModel)
			resp, errAPI := callHostAPICallWithToken(token, "POST", xaiOfficialAPIChatURL, []byte(body), true, false)
			if errAPI != nil {
				return fmt.Errorf("base_url 已写入，但 api.x.ai 复检失败: %w", errAPI)
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("base_url 已写入，但 api.x.ai 复检 HTTP %d", resp.StatusCode)
			}
		}
	}

	// Update in-memory inspection result.
	engine.mu.Lock()
	defer engine.mu.Unlock()
	for i := range engine.results {
		if !resultIdentityMatch(engine.results[i], item) {
			continue
		}
		engine.results[i].AuthBaseURL = target
		engine.results[i].UsingAPI = true
		engine.results[i].PreferredBaseURL = target
		engine.results[i].Classification = "api_gateway_ok"
		engine.results[i].Action = "keep"
		engine.results[i].GatewayNote = "applied_using_api_base_url"
		engine.results[i].Reason = "已写入 using_api=true 与 api.x.ai base_url（现有 token，无需重登）；CPA 原生 grok-4.5 需支持 using_api 的版本"
		if engine.results[i].APIHTTPStatus == 0 {
			engine.results[i].APIHTTPStatus = 200
		}
		engine.bumpResultsLocked()
		return nil
	}
	// Not found in results — still success for file write.
	return nil
}

