package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"grok-inspection/cpasdk/pluginabi"
	"grok-inspection/cpasdk/pluginapi"
)

const (
	// xAI /v1/models currently only advertises grok-4.5; requesting that ID is remapped
	// upstream to grok-4.5-build-free for free accounts. Direct model id grok-4.5-build-free returns 404.
	defaultProbeModel = "grok-4.5"
	xaiResponsesURL       = "https://cli-chat-proxy.grok.com/v1/responses"
	xaiChatCompletionsURL = "https://cli-chat-proxy.grok.com/v1/chat/completions"
	// Official API OpenAI-compatible chat endpoint (no CLI identity headers).
	xaiOfficialAPIChatURL = "https://api.x.ai/v1/chat/completions"
	xaiCLIBaseURL         = "https://cli-chat-proxy.grok.com/v1"
	xaiOfficialAPIBaseURL = "https://api.x.ai/v1"
	xaiInspectionClientVersion = "0.2.93"

	// host.http.do has no native timeout; wrap each call so one bad account cannot stall the whole job.
	// Single host.http.do wall clock. 25s absorbs slow free-tier responses under concurrency.
	probeHTTPTimeout = 25 * time.Second
	// Whole-account budget: CLI (+ optional fallback) and optional api.x.ai probe.
	accountProbeTimeout = 90 * time.Second
	probeTimeoutRetries = 1
	probeTimeoutBackoff = 400 * time.Millisecond
)

type apiCallResponse struct {
	StatusCode int                 `json:"status_code"`
	Header     map[string][]string `json:"header"`
	Body       string              `json:"body"`
}

type probeOutcome struct {
	Response   apiCallResponse
	Error      probeError
	Classified classifyResult
}

// resolveSharedProbeModel always returns the fixed probe model.
// We do not call /v1/models: the list is currently just grok-4.5 and listing adds latency/risk of hang.
func resolveSharedProbeModel(_ []pluginapi.HostAuthFileEntry) string {
	return defaultProbeModel
}

func inspectAccount(file pluginapi.HostAuthFileEntry, model string) accountResult {
	// Hard deadline for the whole account so wg.Wait cannot stick forever.
	type done struct{ result accountResult }
	ch := make(chan done, 1)
	go func() {
		ch <- done{result: inspectAccountInner(file, model)}
	}()
	select {
	case d := <-ch:
		return d.result
	case <-time.After(accountProbeTimeout):
		name := firstNonEmpty(file.Email, file.Label, file.Name, file.AuthIndex, file.ID)
		base := accountResult{
			AuthIndex:      file.AuthIndex,
			Name:           name,
			FileName:       file.Name,
			Email:          file.Email,
			FileID:         file.ID,
			FileSize:       file.Size,
			Disabled:       file.Disabled || isDisabledEntry(file.Disabled, file.Status),
			Model:          firstNonEmpty(model, defaultProbeModel),
			Classification: "probe_error",
			Action:         "keep",
			Reason:         fmt.Sprintf("探测超时（>%s）", accountProbeTimeout),
			ErrorMessage:   "account probe timeout",
		}
		if !file.ModTime.IsZero() {
			base.FileModUnix = file.ModTime.Unix()
		}
		return base
	}
}

func inspectAccountInner(file pluginapi.HostAuthFileEntry, model string) accountResult {
	name := firstNonEmpty(file.Email, file.Label, file.Name, file.AuthIndex, file.ID)
	base := accountResult{
		AuthIndex: file.AuthIndex,
		Name:      name,
		FileName:  file.Name,
		Email:     file.Email,
		FileID:    file.ID,
		FileSize:  file.Size,
		Disabled:  file.Disabled || isDisabledEntry(file.Disabled, file.Status),
	}
	if !file.ModTime.IsZero() {
		base.FileModUnix = file.ModTime.Unix()
	}
	if strings.TrimSpace(file.AuthIndex) == "" {
		base.Classification = "probe_error"
		base.Action = "keep"
		base.Reason = "缺少 auth_index"
		return base
	}

	if strings.TrimSpace(model) == "" {
		model = defaultProbeModel
	}
	base.Model = model

	// Load auth JSON once for token + base_url / using_api metadata.
	authMeta, errMeta := loadAuthProbeMeta(file.AuthIndex)
	if errMeta != nil {
		classified := classifyProbe(classifyInput{
			Disabled:     base.Disabled,
			RequestError: errMeta.Error(),
		})
		base.Classification = classified.Classification
		base.Action = classified.Action
		base.Reason = classified.Reason
		base.ErrorMessage = errMeta.Error()
		return base
	}
	base.AuthBaseURL = authMeta.BaseURL
	base.UsingAPI = authMeta.UsingAPI

	chatBody := fmt.Sprintf(`{"model":%q,"input":"ping","stream":false}`, model)
	chatResp, errChat := callHostAPICallWithToken(authMeta.Token, http.MethodPost, xaiResponsesURL, []byte(chatBody), true, true)
	if errChat != nil {
		classified := classifyProbe(classifyInput{
			Disabled:     base.Disabled,
			RequestError: errChat.Error(),
		})
		base.Classification = classified.Classification
		base.Action = classified.Action
		base.Reason = classified.Reason
		base.ErrorMessage = errChat.Error()
		return base
	}
	// One short retry on bare 429: concurrent inspection often trips temporary throttling.
	if chatResp.StatusCode == http.StatusTooManyRequests {
		parsed := extractError(chatResp.Body)
		if !isFreeUsageExhausted(parsed.Code, parsed.Message) {
			time.Sleep(350 * time.Millisecond)
			if retryResp, errRetry := callHostAPICallWithToken(authMeta.Token, http.MethodPost, xaiResponsesURL, []byte(chatBody), true, true); errRetry == nil {
				chatResp = retryResp
			}
		}
	}

	outcome := newProbeOutcome(chatResp, base.Disabled)
	// Skip expensive fallback when primary already has a definitive classification.
	if shouldTryFallback(chatResp.StatusCode, outcome.Classified.Classification) {
		fallbackBody := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"stream":false}`, model)
		fallbackResp, errFallback := callHostAPICallWithToken(authMeta.Token, http.MethodPost, xaiChatCompletionsURL, []byte(fallbackBody), true, true)
		if errFallback == nil {
			outcome = resolveProbeOutcome(outcome, newProbeOutcome(fallbackResp, base.Disabled))
		}
	}

	base.CLIHTTPStatus = outcome.Response.StatusCode
	base.HTTPStatus = outcome.Response.StatusCode
	base.Classification = outcome.Classified.Classification
	base.Action = outcome.Classified.Action
	base.Reason = outcome.Classified.Reason
	if outcome.Classified.Classification == "healthy" {
		base.PreferredBaseURL = xaiCLIBaseURL
		base.GatewayNote = "cli_ok"
	} else if outcome.Classified.Classification != "healthy" {
		base.ErrorCode = outcome.Error.Code
		base.ErrorMessage = truncateErrMessage(outcome.Error.Message, 400)
	}

	// Dual gateway: when CLI is permission-denied (402/403 path), probe official api.x.ai
	// with the same access_token (no re-login). Do not override reauth/quota.
	if shouldProbeOfficialAPI(outcome.Classified.Classification, outcome.Response.StatusCode) {
		apiBody := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"max_tokens":8,"stream":false}`, model)
		apiResp, errAPI := callHostAPICallWithToken(authMeta.Token, http.MethodPost, xaiOfficialAPIChatURL, []byte(apiBody), true, false)
		if errAPI == nil {
			base.APIHTTPStatus = apiResp.StatusCode
			apiOutcome := newProbeOutcome(apiResp, base.Disabled)
			if apiOutcome.Classified.Classification == "healthy" {
				// CLI denied but official API works with existing token — no re-login needed.
				base.Classification = "api_gateway_ok"
				base.Action = "switch_base_url"
				base.PreferredBaseURL = xaiOfficialAPIBaseURL
				base.GatewayNote = "cli_denied_api_ok_no_relogin"
				base.Reason = fmt.Sprintf("CLI 权限拒绝 (HTTP %d)，api.x.ai 可用（现有 token，无需重登）", base.CLIHTTPStatus)
				base.ErrorCode = ""
				base.ErrorMessage = ""
				// Keep HTTPStatus as CLI status so filters on 403 still work; preferred is api.
			} else if base.Classification == "permission_denied" {
				base.GatewayNote = fmt.Sprintf("cli_denied_api_http_%d", apiResp.StatusCode)
				base.Reason = base.Reason + fmt.Sprintf("；api.x.ai 亦不可用 (HTTP %d)", apiResp.StatusCode)
			}
		} else if base.Classification == "permission_denied" {
			base.GatewayNote = "cli_denied_api_error"
			base.Reason = base.Reason + "；api.x.ai 探测失败: " + truncateErrMessage(errAPI.Error(), 120)
		}
	} else if base.Classification == "healthy" {
		// already set preferred
	} else if base.AuthBaseURL != "" {
		base.PreferredBaseURL = base.AuthBaseURL
	}

	return base
}

// shouldProbeOfficialAPI reports whether to spend a second request on api.x.ai.
func shouldProbeOfficialAPI(classification string, status int) bool {
	switch classification {
	case "permission_denied":
		return true
	case "reauth", "quota_exhausted", "healthy":
		return false
	default:
		// Ambiguous permission-like statuses only.
		return status == http.StatusForbidden || status == http.StatusPaymentRequired
	}
}

// shouldTryFallback reports whether /chat/completions is worth calling after primary /responses.
func shouldTryFallback(status int, classification string) bool {
	switch classification {
	case "reauth", "quota_exhausted", "permission_denied", "healthy":
		return false
	}
	switch status {
	case http.StatusForbidden, http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusPaymentRequired:
		return true
	default:
		return status == 0 || status >= 500 || classification == "probe_error" || classification == "unknown" || classification == "model_unavailable"
	}
}

func newProbeOutcome(resp apiCallResponse, disabled bool) probeOutcome {
	parsed := extractError(resp.Body)
	return probeOutcome{
		Response: resp,
		Error:    parsed,
		Classified: classifyProbe(classifyInput{
			ChatStatus: resp.StatusCode,
			ChatCode:   parsed.Code,
			ChatError:  parsed.Message,
			Disabled:   disabled,
		}),
	}
}

func resolveProbeOutcome(primary, fallback probeOutcome) probeOutcome {
	switch primary.Classified.Classification {
	case "reauth", "quota_exhausted", "permission_denied":
		if fallback.Classified.Classification == "healthy" {
			primary.Classified.Reason += "；备用接口结果不一致，按主探测结果判定"
		}
		return primary
	default:
		return fallback
	}
}

type authProbeMeta struct {
	Token    string
	BaseURL  string
	UsingAPI bool
}

func loadAuthProbeMeta(authIndex string) (authProbeMeta, error) {
	type outcome struct {
		meta authProbeMeta
		err  error
	}
	ch := make(chan outcome, 1)
	go func() {
		meta, err := loadAuthProbeMetaRaw(authIndex)
		ch <- outcome{meta: meta, err: err}
	}()
	select {
	case out := <-ch:
		return out.meta, out.err
	case <-time.After(probeHTTPTimeout):
		return authProbeMeta{}, fmt.Errorf("读取账号 auth 超时（%s）", probeHTTPTimeout)
	}
}

func loadAuthProbeMetaRaw(authIndex string) (authProbeMeta, error) {
	result, errCall := callHost(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: authIndex})
	if errCall != nil {
		return authProbeMeta{}, errCall
	}
	var resp pluginapi.HostAuthGetResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		return authProbeMeta{}, fmt.Errorf("decode host.auth.get: %w", err)
	}
	var data map[string]any
	if err := json.Unmarshal(resp.JSON, &data); err != nil {
		return authProbeMeta{}, fmt.Errorf("decode auth json: %w", err)
	}
	meta := authProbeMeta{
		BaseURL:  asString(data["base_url"]),
		UsingAPI: asBool(data["using_api"]),
	}
	for _, key := range []string{"access_token", "token", "api_key", "id_token"} {
		if value := asString(data[key]); value != "" {
			meta.Token = value
			break
		}
	}
	if meta.Token == "" {
		return authProbeMeta{}, fmt.Errorf("token not found for auth_index %s", authIndex)
	}
	return meta, nil
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}

func xaiInspectionHeaders(token string, jsonBody bool) http.Header {
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("Accept", "application/json")
	headers.Set("X-XAI-Token-Auth", "xai-grok-cli")
	headers.Set("x-grok-client-version", xaiInspectionClientVersion)
	headers.Set("User-Agent", "xai-grok-workspace/"+xaiInspectionClientVersion)
	if jsonBody {
		headers.Set("Content-Type", "application/json")
	}
	return headers
}

// xaiOfficialAPIHeaders is the minimal set for api.x.ai (no CLI identity headers).
func xaiOfficialAPIHeaders(token string, jsonBody bool) http.Header {
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("Accept", "application/json")
	if jsonBody {
		headers.Set("Content-Type", "application/json")
	}
	return headers
}

func isProbeTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "超时") || strings.Contains(msg, "timeout")
}

// callHostAPICall loads token per call (legacy path used by tests).
func callHostAPICall(authIndex, method, rawURL string, body []byte, jsonBody bool) (apiCallResponse, error) {
	token, errToken := resolveAccessToken(authIndex)
	if errToken != nil {
		return apiCallResponse{}, errToken
	}
	return callHostAPICallWithToken(token, method, rawURL, body, jsonBody, true)
}

func callHostAPICallWithToken(token, method, rawURL string, body []byte, jsonBody bool, cliHeaders bool) (apiCallResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= probeTimeoutRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(probeTimeoutBackoff)
		}
		resp, err := callHostAPICallOnceWithToken(token, method, rawURL, body, jsonBody, cliHeaders)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isProbeTimeoutErr(err) {
			return apiCallResponse{}, err
		}
	}
	return apiCallResponse{}, lastErr
}

func callHostAPICallOnceWithToken(token, method, rawURL string, body []byte, jsonBody bool, cliHeaders bool) (apiCallResponse, error) {
	type outcome struct {
		resp apiCallResponse
		err  error
	}
	ch := make(chan outcome, 1)
	go func() {
		resp, err := callHostAPICallRawWithToken(token, method, rawURL, body, jsonBody, cliHeaders)
		ch <- outcome{resp: resp, err: err}
	}()
	select {
	case out := <-ch:
		return out.resp, out.err
	case <-time.After(probeHTTPTimeout):
		return apiCallResponse{}, fmt.Errorf("HTTP 探测超时（%s）: %s %s", probeHTTPTimeout, method, rawURL)
	}
}

func callHostAPICallRawWithToken(token, method, rawURL string, body []byte, jsonBody bool, cliHeaders bool) (apiCallResponse, error) {
	headers := xaiInspectionHeaders(token, jsonBody)
	if !cliHeaders {
		headers = xaiOfficialAPIHeaders(token, jsonBody)
	}
	result, errCall := callHost(pluginabi.MethodHostHTTPDo, map[string]any{
		"method":  method,
		"url":     rawURL,
		"headers": headers,
		"body":    body,
	})
	if errCall != nil {
		return apiCallResponse{}, errCall
	}
	var resp struct {
		StatusCode int                 `json:"StatusCode"`
		Headers    map[string][]string `json:"Headers"`
		Body       []byte              `json:"Body"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		var alt apiCallResponse
		if errAlt := json.Unmarshal(result, &alt); errAlt == nil {
			return alt, nil
		}
		return apiCallResponse{}, fmt.Errorf("decode host.http.do: %w", err)
	}
	return apiCallResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Headers,
		Body:       string(resp.Body),
	}, nil
}

func resolveAccessToken(authIndex string) (string, error) {
	meta, err := loadAuthProbeMeta(authIndex)
	if err != nil {
		return "", err
	}
	return meta.Token, nil
}

func callHostAuthList() (authListResponse, error) {
	type outcome struct {
		resp authListResponse
		err  error
	}
	ch := make(chan outcome, 1)
	go func() {
		resp, err := callHostAuthListRaw()
		ch <- outcome{resp: resp, err: err}
	}()
	select {
	case out := <-ch:
		return out.resp, out.err
	case <-time.After(30 * time.Second):
		return authListResponse{}, fmt.Errorf("列出账号超时（30s）")
	}
}

func callHostAuthListRaw() (authListResponse, error) {
	result, errCall := callHost(pluginabi.MethodHostAuthList, map[string]any{})
	if errCall != nil {
		return authListResponse{}, errCall
	}
	var resp authListResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		return authListResponse{}, fmt.Errorf("decode host.auth.list: %w", err)
	}
	return resp, nil
}
