package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestRefreshXAITokensSuccess(t *testing.T) {
	old := tokenRefreshDo
	t.Cleanup(func() { tokenRefreshDo = old })
	tokenRefreshDo = func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method=%s", req.Method)
		}
		body, _ := io.ReadAll(req.Body)
		form := string(body)
		if !strings.Contains(form, "grant_type=refresh_token") || !strings.Contains(form, "client_id=") {
			t.Fatalf("form=%s", form)
		}
		payload := `{"access_token":"at-new","refresh_token":"rt-new","expires_in":3600,"token_type":"Bearer"}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(payload)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
	got, err := refreshXAITokens(defaultXAITokenURL, "rt-old")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "at-new" || got.RefreshToken != "rt-new" || got.ExpiresIn != 3600 {
		t.Fatalf("got=%+v", got)
	}
}

func TestRefreshXAITokensKeepsOldRefreshOnEmpty(t *testing.T) {
	old := tokenRefreshDo
	t.Cleanup(func() { tokenRefreshDo = old })
	tokenRefreshDo = func(req *http.Request) (*http.Response, error) {
		payload := `{"access_token":"at-only","expires_in":100}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil
	}
	got, err := refreshXAITokens("", "rt-keep")
	if err != nil {
		t.Fatal(err)
	}
	if got.RefreshToken != "rt-keep" {
		t.Fatalf("refresh_token=%q", got.RefreshToken)
	}
}

func TestRefreshXAITokensError(t *testing.T) {
	old := tokenRefreshDo
	t.Cleanup(func() { tokenRefreshDo = old })
	tokenRefreshDo = func(req *http.Request) (*http.Response, error) {
		payload := `{"error":"invalid_grant","error_description":"token revoked"}`
		return &http.Response{
			StatusCode: 400,
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil
	}
	_, err := refreshXAITokens(defaultXAITokenURL, "bad")
	if err == nil || !strings.Contains(err.Error(), "token revoked") {
		t.Fatalf("err=%v", err)
	}
}

func TestApplyRefreshedTokens(t *testing.T) {
	data := map[string]any{
		"refresh_token": "old-rt",
		"access_token":  "old-at",
		"type":          "xai",
	}
	applyRefreshedTokens(data, refreshTokenResponse{
		AccessToken:  "new-at",
		RefreshToken: "new-rt",
		IDToken:      "id",
		TokenType:    "Bearer",
		ExpiresIn:    120,
	})
	if data["access_token"] != "new-at" || data["refresh_token"] != "new-rt" {
		t.Fatalf("tokens not applied: %+v", data)
	}
	if data["auth_kind"] != "oauth" {
		t.Fatalf("auth_kind=%v", data["auth_kind"])
	}
	if asString(data["last_refresh"]) == "" {
		t.Fatal("last_refresh missing")
	}
	// expired should be near now+120s
	expired := asString(data["expired"])
	if expired == "" {
		t.Fatal("expired missing")
	}
	ts, err := time.Parse(time.RFC3339, expired)
	if err != nil {
		t.Fatal(err)
	}
	if ts.Before(time.Now().UTC()) {
		t.Fatalf("expired in the past: %s", expired)
	}
}

func TestStartReauthRequiresCredentialsAndEligible(t *testing.T) {
	oldEngine := engine
	oldCreds := credentials
	t.Cleanup(func() {
		engine = oldEngine
		credentials = oldCreds
	})
	engine = &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{
			{AuthIndex: "1", Email: "a@x.ai", Classification: "permission_denied", HTTPStatus: 403, FileName: "xai-a@x.ai.json"},
		},
	}
	credentials = &credentialStore{byEmail: map[string]credentialLine{}}
	if err := engine.startReauth(reauthStartRequest{}, "", nil); err == nil || !strings.Contains(err.Error(), "上传") {
		t.Fatalf("want upload required, got %v", err)
	}
	_, _ = credentials.replaceFromContent("a@x.ai----p----s")
	// Mutual exclusion with running inspect
	engine.running = true
	if err := engine.startReauth(reauthStartRequest{}, "", nil); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("want busy, got %v", err)
	}
	engine.running = false
	// Healthy-only upload match but not eligible classification
	engine.results = []accountResult{
		{AuthIndex: "1", Email: "a@x.ai", Classification: "healthy", HTTPStatus: 200},
	}
	if err := engine.startReauth(reauthStartRequest{}, "", nil); err == nil || !strings.Contains(err.Error(), "没有可刷新") {
		t.Fatalf("want no eligible, got %v", err)
	}
}

func TestMintXAIViaSSODeviceSuccess(t *testing.T) {
	old := tokenRefreshDo
	t.Cleanup(func() { tokenRefreshDo = old })

	// Sequence: device/code → verify 303 → approve 303 → token 200
	// Also allow optional grok session GET.
	step := 0
	tokenRefreshDo = func(req *http.Request) (*http.Response, error) {
		u := req.URL.String()
		switch {
		case strings.Contains(u, "/oauth2/device/code"):
			step++
			body := `{"device_code":"dc1","user_code":"AB12-CD34","verification_uri":"https://accounts.x.ai/oauth2/device","verification_uri_complete":"https://accounts.x.ai/oauth2/device?user_code=AB12-CD34","expires_in":1800,"interval":5}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
		case strings.Contains(u, "/oauth2/device/verify"):
			step++
			h := http.Header{}
			h.Set("Location", "https://accounts.x.ai/oauth2/device/consent?user_code=AB12-CD34")
			return &http.Response{StatusCode: 303, Body: io.NopCloser(strings.NewReader("")), Header: h}, nil
		case strings.Contains(u, "/oauth2/device/approve"):
			step++
			// Ensure action=allow was posted
			b, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(b), "action=allow") {
				t.Fatalf("approve form=%s", b)
			}
			h := http.Header{}
			h.Set("Location", "https://accounts.x.ai/oauth2/device/done")
			return &http.Response{StatusCode: 303, Body: io.NopCloser(strings.NewReader("")), Header: h}, nil
		case strings.Contains(u, "/oauth2/token"):
			step++
			body := `{"access_token":"at-mint","refresh_token":"rt-mint","id_token":"id-mint","expires_in":3600,"token_type":"Bearer"}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
		case strings.Contains(u, "grok.com/api/auth/session"):
			body := `{"status":"authenticated","session":{"userId":"sub-1","email":"a@x.ai"}}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
		default:
			t.Fatalf("unexpected url %s", u)
			return nil, nil
		}
	}
	got, err := mintXAIViaSSODevice("sso-jwt-value", "sub-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "at-mint" || got.RefreshToken != "rt-mint" {
		t.Fatalf("got=%+v", got)
	}
	if step < 4 {
		t.Fatalf("expected full device flow, steps=%d", step)
	}
}

func TestCredentialsUploadRoute(t *testing.T) {
	oldCreds := credentials
	oldEngine := engine
	t.Cleanup(func() {
		credentials = oldCreds
		engine = oldEngine
	})
	credentials = &credentialStore{byEmail: map[string]credentialLine{}}
	engine = &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{
			{AuthIndex: "9", Email: "u@x.ai", Classification: "reauth", HTTPStatus: 401, FileName: "xai-u@x.ai.json"},
		},
	}
	body, _ := json.Marshal(map[string]string{
		"content": "u@x.ai----secret----sso-token\nbad\n",
	})
	resp := dispatchManagement(pluginapiManagementPOST("/credentials", body))
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", resp.StatusCode, resp.Body)
	}
	if !strings.Contains(string(resp.Body), `"eligible":1`) {
		t.Fatalf("body=%s", resp.Body)
	}
	// Secrets must not appear in response
	if strings.Contains(string(resp.Body), "secret") || strings.Contains(string(resp.Body), "sso-token") {
		t.Fatalf("secrets leaked in response: %s", resp.Body)
	}
	// status light includes credentials summary, no secrets
	st := dispatchManagement(pluginapiManagementGET("/status?include_results=0"))
	if !strings.Contains(string(st.Body), `"has_data":true`) {
		t.Fatalf("status missing credentials: %s", st.Body)
	}
	if strings.Contains(string(st.Body), "secret") {
		t.Fatal("secret in status")
	}
}

// Local helpers avoid importing test plumbing into production.
func pluginapiManagementPOST(suffix string, body []byte) pluginapi.ManagementRequest {
	return pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection" + strings.Split(suffix, "?")[0],
		Body:   body,
	}
}

func pluginapiManagementGET(suffix string) pluginapi.ManagementRequest {
	path := "/v0/management/plugins/grok-inspection" + strings.Split(suffix, "?")[0]
	req := pluginapi.ManagementRequest{Method: http.MethodGet, Path: path}
	if i := strings.Index(suffix, "?"); i >= 0 {
		q := url.Values{}
		for _, part := range strings.Split(suffix[i+1:], "&") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				q.Set(kv[0], kv[1])
			}
		}
		req.Query = q
	}
	return req
}
