package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"grok-inspection/cpasdk/pluginabi"
	"grok-inspection/cpasdk/pluginapi"
)

const (
	xaiOAuthClientID     = "b1a00492-073a-47ea-816f-4c329264a828"
	defaultXAITokenURL   = "https://auth.x.ai/oauth2/token"
	xaiDeviceCodeURL     = "https://auth.x.ai/oauth2/device/code"
	xaiDeviceVerifyURL   = "https://auth.x.ai/oauth2/device/verify"
	xaiDeviceApproveURL  = "https://auth.x.ai/oauth2/device/approve"
	xaiOAuthScope        = "openid profile email offline_access grok-cli:access api:access"
	grokSessionURL       = "https://grok.com/api/auth/session"
	defaultXAIBaseURL    = "https://cli-chat-proxy.grok.com/v1"
	defaultXAIRedirect   = "http://localhost:1455/auth/callback"
	defaultReauthWorkers = 4
	maxReauthWorkers     = 8
	reauthHTTPTimeout    = 30 * time.Second
)

// tokenRefreshDo is the HTTP transport seam for unit tests (also used by device mint).
var tokenRefreshDo = func(req *http.Request) (*http.Response, error) {
	client := &http.Client{
		Timeout: reauthHTTPTimeout,
		// Device verify/approve return 303; do not follow — we read Location ourselves.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
	}
	return client.Do(req)
}

type reauthStartRequest struct {
	AuthIndexes []string `json:"auth_indexes"`
	Workers     int      `json:"workers"`
}

type reauthJobSnapshot struct {
	Running   bool     `json:"running"`
	Done      int      `json:"done"`
	Total     int      `json:"total"`
	Successes int      `json:"successes"`
	Current   string   `json:"current,omitempty"`
	Failures  []string `json:"failures,omitempty"`
	StartedAt string   `json:"started_at,omitempty"`
	FinishedAt string  `json:"finished_at,omitempty"`
}

type refreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func (e *inspectionEngine) startReauth(req reauthStartRequest, _ string, _ http.Header) error {
	workers := req.Workers
	if workers == 0 {
		workers = defaultReauthWorkers
	}
	if workers < 1 || workers > maxReauthWorkers {
		return fmt.Errorf("workers must be between 1 and %d", maxReauthWorkers)
	}
	if !credentials.hasData() {
		return fmt.Errorf("请先上传账号文件（邮箱----密码----sso）")
	}

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

		targets := credentials.matchedEligible(results)
	if len(req.AuthIndexes) > 0 {
		want := map[string]struct{}{}
		for _, key := range req.AuthIndexes {
			key = strings.TrimSpace(key)
			if key != "" {
				want[key] = struct{}{}
			}
		}
		filtered := make([]accountResult, 0, len(targets))
		for _, item := range targets {
			key := firstNonEmpty(item.AuthIndex, item.FileName, item.Name, item.Email)
			if _, ok := want[key]; ok {
				filtered = append(filtered, item)
				continue
			}
			if _, ok := want[item.AuthIndex]; ok {
				filtered = append(filtered, item)
			}
		}
		targets = filtered
	}
	if len(targets) == 0 {
		return fmt.Errorf("没有可刷新的匹配账号（需同时满足：上传文件精确匹配邮箱，且结果为 403/需重登）")
	}

		e.mu.Lock()
		if e.running || e.applying || e.reauthing || e.baseURLApplying {
			e.mu.Unlock()
			return fmt.Errorf("inspection already running")
		}
		e.reauthing = true
		e.reauthStop = false
		e.reauthDone = 0
		e.reauthTotal = len(targets)
		e.reauthSuccesses = 0
		e.reauthCurrent = ""
		e.reauthFailures = nil
		e.reauthStartedAt = time.Now()
		e.reauthFinishedAt = time.Time{}
		e.mu.Unlock()

	e.runWG.Add(1)
	go func() {
		defer e.runWG.Done()
		e.runReauth(targets, workers)
	}()
	return nil
}

func (e *inspectionEngine) stopReauth() {
	e.mu.Lock()
	e.reauthStop = true
	e.mu.Unlock()
}

func (e *inspectionEngine) runReauth(targets []accountResult, workers int) {
	defer func() {
		e.mu.Lock()
		e.reauthing = false
		e.reauthCurrent = ""
		e.reauthStop = false
		e.reauthFinishedAt = time.Now()
		e.persistLocked()
		e.mu.Unlock()
	}()

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := range targets {
		e.mu.Lock()
		stop := e.reauthStop
		e.mu.Unlock()
		if stop {
			break
		}
		item := targets[i]
		wg.Add(1)
		sem <- struct{}{}
		go func(item accountResult) {
			defer wg.Done()
			defer func() { <-sem }()
			e.mu.Lock()
			if e.reauthStop {
				e.mu.Unlock()
				return
			}
			e.reauthCurrent = firstNonEmpty(item.Email, item.Name, item.FileName, item.AuthIndex)
			e.mu.Unlock()

			err := refreshAndReprobeAccount(item)
			e.mu.Lock()
			e.reauthDone++
			if err != nil {
				e.reauthFailures = append(e.reauthFailures, firstNonEmpty(item.Email, item.Name)+": "+err.Error())
			} else {
				e.reauthSuccesses++
			}
			e.mu.Unlock()
		}(item)
	}
	wg.Wait()
}

func refreshAndReprobeAccount(item accountResult) error {
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
		return fmt.Errorf("auth 文件名无效: %s", fileName)
	}

	email := firstNonEmpty(item.Email, asString(data["email"]), resultCredentialEmail(item))
	cred, hasCred := credentials.lookupExact(email)
	if !hasCred {
		// Try exact match via file-name style keys already in the upload map.
		for _, candidate := range []string{email, resultCredentialEmail(item)} {
			if candidate == "" {
				continue
			}
			if c, ok := credentials.lookupExact(candidate); ok {
				cred, hasCred = c, true
				email = candidate
				break
			}
		}
	}

	method := ""
	var tokens refreshTokenResponse
	var errMint error

	// Prefer pure-HTTP device mint with SSO cookie (same path as grok-auto-register, no browser).
	if hasCred && strings.TrimSpace(cred.SSO) != "" {
		tokens, errMint = mintXAIViaSSODevice(strings.TrimSpace(cred.SSO), asString(data["sub"]))
		if errMint == nil {
			method = "sso_device"
		}
	}

	// Fallback: silent refresh_token (works when token expired, not when account is banned).
	if method == "" {
		refreshToken := asString(data["refresh_token"])
		if refreshToken == "" {
			if errMint != nil {
				return fmt.Errorf("SSO 重登失败且无 refresh_token: %v", errMint)
			}
			return fmt.Errorf("缺少 SSO 与 refresh_token，无法静默登录")
		}
		tokenURL := firstNonEmpty(asString(data["token_endpoint"]), defaultXAITokenURL)
		var errRefresh error
		tokens, errRefresh = refreshXAITokens(tokenURL, refreshToken)
		if errRefresh != nil {
			if errMint != nil {
				return fmt.Errorf("SSO 重登失败 (%v)；refresh_token 也失败 (%v)", errMint, errRefresh)
			}
			return errRefresh
		}
		method = "refresh_token"
		if errMint != nil {
			// Keep SSO error as context only; refresh succeeded.
			_ = errMint
		}
	}

	applyRefreshedTokens(data, tokens)
	if email != "" {
		data["email"] = email
	}
	ensureDefaultXAIAuthFields(data)
	raw, errMarshal := json.Marshal(data)
	if errMarshal != nil {
		return errMarshal
	}
	if errSave := callHostAuthSave(fileName, raw); errSave != nil {
		return errSave
	}

	// Re-probe using the same host path as inspection (token is re-read from auth file).
	entry := pluginapi.HostAuthFileEntry{
		AuthIndex: authIndex,
		Name:      fileName,
		ID:        firstNonEmpty(item.FileID, fileName),
		Email:     firstNonEmpty(email, asString(data["email"])),
		Disabled:  item.Disabled,
		Size:      item.FileSize,
	}
	if item.FileModUnix > 0 {
		entry.ModTime = time.Unix(item.FileModUnix, 0)
	}
	result := inspectAccount(entry, defaultProbeModel)
	engine.mu.Lock()
	if idx := findResultIndex(engine.results, result); idx >= 0 {
		if result.FileName == "" {
			result.FileName = engine.results[idx].FileName
		}
		if result.Email == "" {
			result.Email = engine.results[idx].Email
		}
		engine.results[idx] = result
	} else {
		engine.results = append(engine.results, result)
	}
	engine.bumpResultsLocked()
	engine.persistLocked()
	engine.mu.Unlock()

	if result.Classification == "healthy" {
		return nil
	}
	// Token write succeeded but account still unhealthy — surface as soft failure for UI.
	return fmt.Errorf("%s 已写入，复检仍为 %s（HTTP %d）: %s",
		method, result.Classification, result.HTTPStatus, firstNonEmpty(result.Reason, result.ErrorMessage, "unknown"))
}

// mintXAIViaSSODevice performs the CPA/Grok device OAuth flow over pure HTTP:
// device/code → device/verify (SSO cookie) → device/approve (action=allow) → token poll.
// No browser / Turnstile is required when a valid sso cookie JWT is available.
func mintXAIViaSSODevice(sso, preferredSub string) (refreshTokenResponse, error) {
	sso = strings.TrimSpace(sso)
	if sso == "" {
		return refreshTokenResponse{}, fmt.Errorf("empty sso")
	}
	cookie := "sso=" + sso + "; sso-rw=" + sso

	// 1) device code
	form := url.Values{}
	form.Set("client_id", xaiOAuthClientID)
	form.Set("scope", xaiOAuthScope)
	status, body, _, err := httpForm(http.MethodPost, xaiDeviceCodeURL, form, map[string]string{
		"Accept": "application/json",
		"Origin": "https://auth.x.ai",
	})
	if err != nil {
		return refreshTokenResponse{}, err
	}
	if status < 200 || status >= 300 {
		return refreshTokenResponse{}, fmt.Errorf("device/code HTTP %d: %s", status, trimForErr(body))
	}
	var dev struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}
	if err := json.Unmarshal(body, &dev); err != nil {
		return refreshTokenResponse{}, fmt.Errorf("decode device/code: %w", err)
	}
	if dev.DeviceCode == "" || dev.UserCode == "" {
		return refreshTokenResponse{}, fmt.Errorf("device/code missing fields")
	}

	// 2) verify with SSO cookie → 303 to consent
	vForm := url.Values{}
	vForm.Set("user_code", dev.UserCode)
	status, _, hdr, err := httpForm(http.MethodPost, xaiDeviceVerifyURL, vForm, map[string]string{
		"Cookie":  cookie,
		"Origin":  "https://accounts.x.ai",
		"Referer": "https://accounts.x.ai/oauth2/device",
		"Accept":  "text/html,application/json,*/*",
	})
	if err != nil {
		return refreshTokenResponse{}, err
	}
	if status != http.StatusSeeOther && status != http.StatusFound && status != http.StatusTemporaryRedirect && status != http.StatusOK {
		return refreshTokenResponse{}, fmt.Errorf("device/verify HTTP %d (SSO 可能失效)", status)
	}
	consentURL := strings.TrimSpace(hdr.Get("Location"))
	if consentURL == "" {
		consentURL = "https://accounts.x.ai/oauth2/device/consent?user_code=" + url.QueryEscape(dev.UserCode)
	}

	// 3) resolve principal_id (sub) for approve form
	principalID := strings.TrimSpace(preferredSub)
	if principalID == "" {
		principalID = lookupGrokSessionUserID(cookie)
	}

	// 4) approve
	aForm := url.Values{}
	aForm.Set("user_code", dev.UserCode)
	aForm.Set("action", "allow")
	aForm.Set("principal_type", "User")
	aForm.Set("principal_id", principalID)
	status, _, _, err = httpForm(http.MethodPost, xaiDeviceApproveURL, aForm, map[string]string{
		"Cookie":  cookie,
		"Origin":  "https://accounts.x.ai",
		"Referer": consentURL,
		"Accept":  "text/html,application/json,*/*",
	})
	if err != nil {
		return refreshTokenResponse{}, err
	}
	if status != http.StatusSeeOther && status != http.StatusFound && status != http.StatusOK && status != http.StatusTemporaryRedirect {
		return refreshTokenResponse{}, fmt.Errorf("device/approve HTTP %d", status)
	}

	// 5) exchange device_code for tokens
	tForm := url.Values{}
	tForm.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	tForm.Set("device_code", dev.DeviceCode)
	tForm.Set("client_id", xaiOAuthClientID)
	status, body, _, err = httpForm(http.MethodPost, defaultXAITokenURL, tForm, map[string]string{
		"Accept": "application/json",
		"Origin": "https://auth.x.ai",
	})
	if err != nil {
		return refreshTokenResponse{}, err
	}
	var parsed refreshTokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return refreshTokenResponse{}, fmt.Errorf("decode device token HTTP %d: %w", status, err)
	}
	if status < 200 || status >= 300 || parsed.AccessToken == "" {
		msg := firstNonEmpty(parsed.ErrorDesc, parsed.Error, trimForErr(body))
		return refreshTokenResponse{}, fmt.Errorf("device token 失败: %s", msg)
	}
	if parsed.RefreshToken == "" {
		return refreshTokenResponse{}, fmt.Errorf("device token 缺少 refresh_token")
	}
	if parsed.ExpiresIn <= 0 {
		parsed.ExpiresIn = 21600
	}
	return parsed, nil
}

func lookupGrokSessionUserID(cookie string) string {
	req, err := http.NewRequest(http.MethodGet, grokSessionURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://grok.com")
	req.Header.Set("Referer", "https://grok.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, errDo := tokenRefreshDo(req)
	if errDo != nil {
		return ""
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var sess struct {
		Status  string `json:"status"`
		Session struct {
			UserID string `json:"userId"`
			Email  string `json:"email"`
		} `json:"session"`
	}
	if err := json.Unmarshal(raw, &sess); err != nil {
		return ""
	}
	return strings.TrimSpace(sess.Session.UserID)
}

func httpForm(method, endpoint string, form url.Values, headers map[string]string) (int, []byte, http.Header, error) {
	req, err := http.NewRequest(method, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "grok-inspection-reauth/0.1.12")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, errDo := tokenRefreshDo(req)
	if errDo != nil {
		return 0, nil, nil, errDo
	}
	defer resp.Body.Close()
	raw, errRead := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if errRead != nil {
		return resp.StatusCode, nil, resp.Header.Clone(), errRead
	}
	return resp.StatusCode, raw, resp.Header.Clone(), nil
}

func trimForErr(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}

func ensureDefaultXAIAuthFields(data map[string]any) {
	if asString(data["type"]) == "" {
		data["type"] = "xai"
	}
	if asString(data["auth_kind"]) == "" {
		data["auth_kind"] = "oauth"
	}
	if asString(data["token_endpoint"]) == "" {
		data["token_endpoint"] = defaultXAITokenURL
	}
	if asString(data["base_url"]) == "" {
		data["base_url"] = defaultXAIBaseURL
	}
	if asString(data["redirect_uri"]) == "" {
		data["redirect_uri"] = defaultXAIRedirect
	}
	if data["headers"] == nil {
		data["headers"] = map[string]any{
			"User-Agent":                "grok-shell/0.2.93 (linux; x86_64)",
			"x-authenticateresponse":    "authenticate-response",
			"x-grok-client-identifier":  "grok-shell",
			"x-grok-client-version":     "0.2.93",
			"x-xai-token-auth":          "xai-grok-cli",
		}
	}
}

func loadAuthJSONMap(authIndex string) (fileName string, data map[string]any, err error) {
	result, errCall := callHost(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: authIndex})
	if errCall != nil {
		return "", nil, errCall
	}
	var resp pluginapi.HostAuthGetResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		return "", nil, fmt.Errorf("decode host.auth.get: %w", err)
	}
	if err := json.Unmarshal(resp.JSON, &data); err != nil {
		return "", nil, fmt.Errorf("decode auth json: %w", err)
	}
	name := firstNonEmpty(resp.Name, strings.TrimSpace(resp.Path))
	// host.auth.get may return a full path; save expects a .json file name.
	name = strings.ReplaceAll(name, "\\", "/")
	if base := path.Base(name); base != "" && base != "." && base != "/" {
		name = base
	}
	return name, data, nil
}

func callHostAuthSave(name string, raw json.RawMessage) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("auth file name is required")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		return fmt.Errorf("auth file name must end with .json")
	}
	result, errCall := callHost(pluginabi.MethodHostAuthSave, pluginapi.HostAuthSaveRequest{
		Name: name,
		JSON: raw,
	})
	if errCall != nil {
		return errCall
	}
	var resp pluginapi.HostAuthSaveResponse
	if len(result) > 0 {
		_ = json.Unmarshal(result, &resp)
	}
	return nil
}

func refreshXAITokens(tokenURL, refreshToken string) (refreshTokenResponse, error) {
	tokenURL = strings.TrimSpace(tokenURL)
	if tokenURL == "" {
		tokenURL = defaultXAITokenURL
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", xaiOAuthClientID)

	req, errReq := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if errReq != nil {
		return refreshTokenResponse{}, errReq
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "grok-inspection-reauth/0.1.12")

	resp, errDo := tokenRefreshDo(req)
	if errDo != nil {
		return refreshTokenResponse{}, errDo
	}
	defer resp.Body.Close()
	raw, errRead := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if errRead != nil {
		return refreshTokenResponse{}, errRead
	}
	var parsed refreshTokenResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return refreshTokenResponse{}, fmt.Errorf("decode token response HTTP %d: %w", resp.StatusCode, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || parsed.AccessToken == "" {
		msg := firstNonEmpty(parsed.ErrorDesc, parsed.Error, strings.TrimSpace(string(raw)))
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return refreshTokenResponse{}, fmt.Errorf("refresh_token 失败: %s", msg)
	}
	if parsed.RefreshToken == "" {
		// Some providers rotate only access_token; keep old refresh by caller.
		parsed.RefreshToken = refreshToken
	}
	if parsed.ExpiresIn <= 0 {
		parsed.ExpiresIn = 21600
	}
	return parsed, nil
}

func applyRefreshedTokens(data map[string]any, tokens refreshTokenResponse) {
	data["access_token"] = tokens.AccessToken
	if tokens.RefreshToken != "" {
		data["refresh_token"] = tokens.RefreshToken
	}
	if tokens.IDToken != "" {
		data["id_token"] = tokens.IDToken
	}
	if tokens.TokenType != "" {
		data["token_type"] = tokens.TokenType
	}
	data["expires_in"] = tokens.ExpiresIn
	data["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
	if expired, expIn, sub := expiredFromAccessToken(tokens.AccessToken); expired != "" {
		data["expired"] = expired
		if expIn > 0 {
			data["expires_in"] = expIn
		}
		if sub != "" {
			if asString(data["sub"]) == "" {
				data["sub"] = sub
			}
		}
	} else if tokens.ExpiresIn > 0 {
		data["expired"] = time.Now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	if asString(data["type"]) == "" {
		data["type"] = "xai"
	}
	if asString(data["auth_kind"]) == "" {
		data["auth_kind"] = "oauth"
	}
	if asString(data["token_endpoint"]) == "" {
		data["token_endpoint"] = defaultXAITokenURL
	}
}

func expiredFromAccessToken(accessToken string) (expired string, expiresIn int, sub string) {
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return "", 0, ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Try standard padded decoding.
		raw := parts[1]
		if m := len(raw) % 4; m != 0 {
			raw += strings.Repeat("=", 4-m)
		}
		payload, err = base64.URLEncoding.DecodeString(raw)
		if err != nil {
			return "", 0, ""
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", 0, ""
	}
	exp := int64(0)
	switch v := claims["exp"].(type) {
	case float64:
		exp = int64(v)
	case json.Number:
		exp, _ = v.Int64()
	}
	if exp <= 0 {
		return "", 0, ""
	}
	iat := exp - 21600
	switch v := claims["iat"].(type) {
	case float64:
		iat = int64(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			iat = n
		}
	}
	expired = time.Unix(exp, 0).UTC().Format(time.RFC3339)
	if exp > iat {
		expiresIn = int(exp - iat)
	}
	sub = firstNonEmpty(asString(claims["sub"]), asString(claims["principal_id"]))
	return expired, expiresIn, sub
}

func (e *inspectionEngine) reauthSnapshotLocked() reauthJobSnapshot {
	snap := reauthJobSnapshot{
		Running:    e.reauthing,
		Done:       e.reauthDone,
		Total:      e.reauthTotal,
		Successes:  e.reauthSuccesses,
		Current:    e.reauthCurrent,
		Failures:   append([]string(nil), e.reauthFailures...),
	}
	if !e.reauthStartedAt.IsZero() {
		snap.StartedAt = e.reauthStartedAt.Format(time.RFC3339)
	}
	if !e.reauthFinishedAt.IsZero() {
		snap.FinishedAt = e.reauthFinishedAt.Format(time.RFC3339)
	}
	return snap
}
