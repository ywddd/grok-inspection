package main

import (
	"encoding/json"
	"errors"
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
	defaultProbeModel          = "grok-4.5"
	xaiResponsesURL            = "https://cli-chat-proxy.grok.com/v1/responses"
	xaiChatCompletionsURL      = "https://cli-chat-proxy.grok.com/v1/chat/completions"
	xaiInspectionClientVersion = "0.2.93"

	// host.http.do has no native timeout; wrap each call so one bad account cannot stall the whole job.
	// Single host.http.do wall clock. 25s absorbs slow free-tier responses under concurrency.
	probeHTTPTimeout = 25 * time.Second
	// Whole-account budget allows the primary response probe plus an ambiguous-result
	// fallback, while timeout retries are scheduled separately by the engine.
	accountProbeTimeout = 55 * time.Second
)

var (
	errHTTPProbeTimeout    = errors.New("http_probe_timeout")
	errListAccountsTimeout = errors.New("list_accounts_timeout")
)

// probeHTTPTimeoutError is a soft per-call timeout (probeHTTPTimeout, typically 25s).
// Unwraps to errHTTPProbeTimeout so retry detection stays language-agnostic.
type probeHTTPTimeoutError struct {
	d      time.Duration
	method string
	url    string
}

func (e *probeHTTPTimeoutError) Error() string {
	if e == nil {
		return errHTTPProbeTimeout.Error()
	}
	return fmt.Sprintf("%s: %s %s %s", errHTTPProbeTimeout.Error(), e.d, e.method, e.url)
}

func (e *probeHTTPTimeoutError) Unwrap() error { return errHTTPProbeTimeout }

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

func inspectAccount(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
	// Soft whole-account deadline so the engine can mark timeout and free a worker
	// slot. Abandoned probe work still holds hostCallGate until host.http.do returns.
	type done struct{ result accountResult }
	ch := make(chan done, 1)
	go func() {
		ch <- done{result: inspectAccountInner(file, model, lang)}
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
			Reason:         T(lang, "probe_timeout", accountProbeTimeout),
			ErrorMessage:   "account probe timeout",
		}
		if !file.ModTime.IsZero() {
			base.FileModUnix = file.ModTime.Unix()
		}
		return base
	}
}

func inspectAccountInner(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
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
		base.Reason = T(lang, "missing_auth_index")
		return base
	}

	if strings.TrimSpace(model) == "" {
		model = defaultProbeModel
	}
	base.Model = model

	chatBody := fmt.Sprintf(`{"model":%q,"input":"ping","stream":false}`, model)
	chatResp, errChat := callHostAPICall(file.AuthIndex, http.MethodPost, xaiResponsesURL, []byte(chatBody), true)
	if errChat != nil {
		reqErr := errChat.Error()
		if isProbeTimeoutErr(errChat) {
			base.Classification = "probe_error"
			base.Action = "keep"
			var ht *probeHTTPTimeoutError
			if errors.As(errChat, &ht) && ht != nil {
				// Per-call soft timeout (typically 25s), not the whole-account 55s budget.
				base.Reason = T(lang, "http_probe_timeout", ht.d, ht.method, ht.url)
				base.ErrorMessage = "http probe timeout"
			} else {
				base.Reason = T(lang, "probe_timeout", accountProbeTimeout)
				base.ErrorMessage = "account probe timeout"
			}
			return base
		}
		classified := classifyProbe(classifyInput{
			Lang:         lang,
			Disabled:     base.Disabled,
			RequestError: reqErr,
		})
		base.Classification = classified.Classification
		base.Action = classified.Action
		base.Reason = classified.Reason
		base.ErrorMessage = reqErr
		return base
	}
	// One short retry on bare 429: concurrent inspection often trips temporary throttling.
	if chatResp.StatusCode == http.StatusTooManyRequests {
		parsed := extractError(chatResp.Body)
		if !isFreeUsageExhausted(parsed.Code, parsed.Message) {
			time.Sleep(350 * time.Millisecond)
			if retryResp, errRetry := callHostAPICall(file.AuthIndex, http.MethodPost, xaiResponsesURL, []byte(chatBody), true); errRetry == nil {
				chatResp = retryResp
			}
		}
	}

	outcome := newProbeOutcome(chatResp, base.Disabled, lang)
	// Skip expensive fallback when primary already has a definitive classification.
	if shouldTryFallback(chatResp.StatusCode, outcome.Classified.Classification) {
		fallbackBody := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"stream":false}`, model)
		fallbackResp, errFallback := callHostAPICall(file.AuthIndex, http.MethodPost, xaiChatCompletionsURL, []byte(fallbackBody), true)
		if errFallback == nil {
			outcome = resolveProbeOutcome(outcome, newProbeOutcome(fallbackResp, base.Disabled, lang), lang)
		}
	}

	base.HTTPStatus = outcome.Response.StatusCode
	base.Classification = outcome.Classified.Classification
	base.Action = outcome.Classified.Action
	base.Reason = outcome.Classified.Reason
	// Only keep compact error fields for non-healthy results (export stays readable).
	if outcome.Classified.Classification != "healthy" {
		base.ErrorCode = outcome.Error.Code
		base.ErrorMessage = truncateErrMessage(outcome.Error.Message, 400)
	}
	return base
}

// shouldTryFallback reports whether /chat/completions is worth calling after primary /responses.
// Bare 429 is excluded: free-usage is definitive from primary, and temporary throttle
// already gets one short primary retry. Fallback after 429 often burns the remaining
// account budget (55s) while abandoned host.http.do still holds a gate slot.
func shouldTryFallback(status int, classification string) bool {
	switch classification {
	case "reauth", "quota_exhausted", "permission_denied", "healthy":
		return false
	}
	switch status {
	case http.StatusForbidden, http.StatusUnauthorized, http.StatusPaymentRequired:
		return true
	case http.StatusTooManyRequests:
		return false
	default:
		return status == 0 || status >= 500 || classification == "probe_error" || classification == "unknown" || classification == "model_unavailable"
	}
}

func newProbeOutcome(resp apiCallResponse, disabled bool, lang Lang) probeOutcome {
	parsed := extractError(resp.Body)
	return probeOutcome{
		Response: resp,
		Error:    parsed,
		Classified: classifyProbe(classifyInput{
			Lang:       lang,
			ChatStatus: resp.StatusCode,
			ChatCode:   parsed.Code,
			ChatError:  parsed.Message,
			Disabled:   disabled,
		}),
	}
}

func resolveProbeOutcome(primary, fallback probeOutcome, lang Lang) probeOutcome {
	switch primary.Classified.Classification {
	case "reauth", "quota_exhausted", "permission_denied":
		if fallback.Classified.Classification == "healthy" {
			primary.Classified.Reason += T(lang, "fallback_disagreed")
		}
		return primary
	default:
		return fallback
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

func isProbeTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errHTTPProbeTimeout) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "超时") || strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "http_probe_timeout")
}

// callHostAPICall runs one timed host.http.do. Timeout retries are deferred to
// the engine's low-concurrency retry phase so slow accounts do not occupy all
// primary workers twice.
func callHostAPICall(authIndex, method, rawURL string, body []byte, jsonBody bool) (apiCallResponse, error) {
	return callHostAPICallOnce(authIndex, method, rawURL, body, jsonBody)
}

func callHostAPICallOnce(authIndex, method, rawURL string, body []byte, jsonBody bool) (apiCallResponse, error) {
	// Soft deadline for UI/classification only. The underlying host call is not
	// cancelled; acquireHostCall in callHost keeps abandoned CGO work bounded.
	type outcome struct {
		resp apiCallResponse
		err  error
	}
	ch := make(chan outcome, 1)
	go func() {
		resp, err := callHostAPICallRaw(authIndex, method, rawURL, body, jsonBody)
		ch <- outcome{resp: resp, err: err}
	}()
	select {
	case out := <-ch:
		return out.resp, out.err
	case <-time.After(probeHTTPTimeout):
		return apiCallResponse{}, &probeHTTPTimeoutError{d: probeHTTPTimeout, method: method, url: rawURL}
	}
}

func callHostAPICallRaw(authIndex, method, rawURL string, body []byte, jsonBody bool) (apiCallResponse, error) {
	token, errToken := resolveAccessToken(authIndex)
	if errToken != nil {
		return apiCallResponse{}, errToken
	}
	result, errCall := callHost(pluginabi.MethodHostHTTPDo, map[string]any{
		"method":  method,
		"url":     rawURL,
		"headers": xaiInspectionHeaders(token, jsonBody),
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
	// No nested soft-timeout here: callHost already gates CGO concurrency, and
	// callHostAPICallOnce provides the HTTP-level soft deadline for the whole probe.
	return resolveAccessTokenRaw(authIndex)
}

func resolveAccessTokenRaw(authIndex string) (string, error) {
	result, errCall := callHost(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: authIndex})
	if errCall != nil {
		return "", errCall
	}
	var resp pluginapi.HostAuthGetResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		return "", fmt.Errorf("decode host.auth.get: %w", err)
	}
	var data map[string]any
	if err := json.Unmarshal(resp.JSON, &data); err != nil {
		return "", fmt.Errorf("decode auth json: %w", err)
	}
	for _, key := range []string{"access_token", "token", "api_key", "id_token"} {
		if value := asString(data[key]); value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("token not found for auth_index %s", authIndex)
}

func callHostAuthList() (authListResponse, error) {
	// Soft list deadline only; callHost gates the real CGO host.auth.list call.
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
		return authListResponse{}, errListAccountsTimeout
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
