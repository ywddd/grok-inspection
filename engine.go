package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"grok-inspection/cpasdk/pluginabi"
	"grok-inspection/cpasdk/pluginapi"
)

const (
	defaultWorkers      = 6
	minWorkers          = 1
	maxWorkers          = 16
	defaultApplyWorkers = 6 // concurrent Management API calls for bulk enable/disable
	maxApplyWorkers     = 8
	applyPersistEvery   = 25 // persist every N bulk ops (not each) for speed
	// CPA DELETE /auth-files supports multi-name in one request. 50 balances
	// payload size, partial-failure reporting, and progress granularity.
	deleteBatchSize = 50
)

type accountResult struct {
	AuthIndex      string `json:"auth_index"`
	Name           string `json:"name"`
	FileName       string `json:"file_name,omitempty"`
	Email          string `json:"email,omitempty"`
	Disabled       bool   `json:"disabled"`
	Classification string `json:"classification"`
	Action         string `json:"action"`
	Reason         string `json:"reason"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	Model          string `json:"model,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

type jobSnapshot struct {
	Running         bool            `json:"running"`
	Stopped         bool            `json:"stopped"`
	Applying        bool            `json:"applying"`
	Incremental     bool            `json:"incremental"`
	Done            int             `json:"done"`
	Total           int             `json:"total"`
	Workers         int             `json:"workers"`
	IncludeDisabled bool            `json:"include_disabled"`
	OnlyDisabled    bool            `json:"only_disabled"`
	ApplyDone       int             `json:"apply_done"`
	ApplyTotal      int             `json:"apply_total"`
	ApplyCurrent    string          `json:"apply_current,omitempty"`
	ApplyFailures   []string        `json:"apply_failures,omitempty"`
	StartedAt       string          `json:"started_at,omitempty"`
	FinishedAt      string          `json:"finished_at,omitempty"`
	Results         []accountResult `json:"results,omitempty"`
	Summary         map[string]int  `json:"summary"`
	StorePath       string          `json:"store_path,omitempty"`
	// ResultsGen bumps whenever results content changes; light status omits Results.
	ResultsGen     uint64 `json:"results_gen"`
	IncludeResults bool   `json:"include_results"`
}

type startRequest struct {
	Workers         int  `json:"workers"`
	IncludeDisabled bool `json:"include_disabled"`
	OnlyDisabled    bool `json:"only_disabled"`
	// Incremental only probes Auth accounts not already present in the last results.
	Incremental bool `json:"incremental"`
}

type applyRequest struct {
	// empty AuthIndexes means apply all matching recommended actions (when ForceAction empty)
	AuthIndexes     []string `json:"auth_indexes"`
	Actions         []string `json:"actions"`         // optional: disable/enable/delete (recommended only)
	Classifications []string `json:"classifications"` // optional: reauth/healthy/...
	// ForceAction overrides recommended action for selected accounts.
	// Used by filter-based bulk disable/delete. Values: disable | enable | delete
	ForceAction string `json:"force_action"`
}

type actionRequest struct {
	AuthIndex string `json:"auth_index"`
	Name      string `json:"name"`
	Disabled  bool   `json:"disabled"`
	Delete    bool   `json:"delete"`
}

type authListResponse struct {
	Files []pluginapi.HostAuthFileEntry `json:"files"`
}

type inspectionEngine struct {
	mu              sync.Mutex
	runWG           sync.WaitGroup
	running         bool
	stopped         bool
	applying        bool
	actionInFlight  int // concurrent single-row enable/disable/delete goroutines
	incremental     bool
	runID           uint64
	workers         int
	includeDisabled bool
	onlyDisabled    bool
	total           int
	probeDone       int // probes completed in the current run (full or incremental)
	results         []accountResult
	applyDone       int
	applyTotal      int
	applyCurrent    string
	applyFailures   []string
	resultsGen      uint64 // monotonic; used by light /status clients
	startedAt       time.Time
	finishedAt      time.Time
}

var engine = &inspectionEngine{workers: defaultWorkers}

func init() {
	engine.loadFromDisk()
}

func normalizeWorkers(workers int) (int, error) {
	if workers == 0 {
		return defaultWorkers, nil
	}
	if workers < minWorkers || workers > maxWorkers {
		return 0, fmt.Errorf("workers must be an integer between %d and %d", minWorkers, maxWorkers)
	}
	return workers, nil
}

func (e *inspectionEngine) loadFromDisk() {
	snap, err := loadPersistedSnapshot()
	if err != nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running || e.applying {
		return
	}
	e.results = append([]accountResult(nil), snap.Results...)
	e.bumpResultsLocked()
	if snap.Workers >= minWorkers && snap.Workers <= maxWorkers {
		e.workers = snap.Workers
	}
	e.includeDisabled = snap.IncludeDisabled
	e.onlyDisabled = snap.OnlyDisabled
	e.total = len(snap.Results)
	if snap.StartedAt != "" {
		if t, errParse := time.Parse(time.RFC3339, snap.StartedAt); errParse == nil {
			e.startedAt = t
		}
	}
	if snap.FinishedAt != "" {
		if t, errParse := time.Parse(time.RFC3339, snap.FinishedAt); errParse == nil {
			e.finishedAt = t
		}
	}
}

func (e *inspectionEngine) persistLocked() {
	snap := persistedSnapshot{
		Workers:         e.workers,
		IncludeDisabled: e.includeDisabled,
		OnlyDisabled:    e.onlyDisabled,
		Results:         append([]accountResult(nil), e.results...),
	}
	if !e.startedAt.IsZero() {
		snap.StartedAt = e.startedAt.Format(time.RFC3339)
	}
	if !e.finishedAt.IsZero() {
		snap.FinishedAt = e.finishedAt.Format(time.RFC3339)
	}
	// Best-effort; status API must never fail because of disk errors.
	_ = savePersistedSnapshot(snap)
}

func (e *inspectionEngine) persist() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.persistLocked()
}

func (e *inspectionEngine) bumpResultsLocked() {
	e.resultsGen++
}

func summarizeResults(results []accountResult) map[string]int {
	summary := map[string]int{
		"total":             len(results),
		"healthy":           0,
		"permission_denied": 0,
		"quota_exhausted":   0,
		"reauth":            0,
		"other":             0,
	}
	for _, item := range results {
		switch item.Classification {
		case "healthy":
			summary["healthy"]++
		case "permission_denied":
			summary["permission_denied"]++
		case "quota_exhausted":
			summary["quota_exhausted"]++
		case "reauth":
			summary["reauth"]++
		default:
			summary["other"]++
		}
	}
	return summary
}

// snapshot builds a status payload. When includeResults is false (light poll),
// Results is omitted so progress polling stays cheap with 1000+ accounts.
func (e *inspectionEngine) snapshot(includeResults bool) jobSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshotLocked(includeResults)
}

func (e *inspectionEngine) snapshotLocked(includeResults bool) jobSnapshot {
	summary := summarizeResults(e.results)
	snap := jobSnapshot{
		Running:         e.running,
		Stopped:         e.stopped && !e.running,
		Applying:        e.applying,
		Incremental:     e.incremental,
		Done:            e.probeDone,
		Total:           e.total,
		Workers:         e.workers,
		IncludeDisabled: e.includeDisabled,
		OnlyDisabled:    e.onlyDisabled,
		ApplyDone:       e.applyDone,
		ApplyTotal:      e.applyTotal,
		ApplyCurrent:    e.applyCurrent,
		ApplyFailures:   append([]string(nil), e.applyFailures...),
		Summary:         summary,
		StorePath:       storeFilePath(),
		ResultsGen:      e.resultsGen,
		IncludeResults:  includeResults,
	}
	if includeResults {
		snap.Results = append([]accountResult(nil), e.results...)
	}
	if !e.startedAt.IsZero() {
		snap.StartedAt = e.startedAt.Format(time.RFC3339)
	}
	if !e.finishedAt.IsZero() {
		snap.FinishedAt = e.finishedAt.Format(time.RFC3339)
	}
	return snap
}

func (e *inspectionEngine) start(req startRequest) error {
	workers, errWorkers := normalizeWorkers(req.Workers)
	if errWorkers != nil {
		return errWorkers
	}

	e.mu.Lock()
	if e.running || e.applying {
		e.mu.Unlock()
		return fmt.Errorf("inspection already running")
	}
	if e.actionInFlight > 0 {
		e.mu.Unlock()
		return fmt.Errorf("busy: row action in progress")
	}
	if req.Incremental && len(e.results) == 0 {
		e.mu.Unlock()
		return fmt.Errorf("增量巡检需要已有结果，请先完整巡检")
	}
	includeDisabled := req.IncludeDisabled
	onlyDisabled := req.OnlyDisabled
	if onlyDisabled {
		includeDisabled = false
	}
	e.running = true
	e.stopped = false
	e.applying = false
	e.incremental = req.Incremental
	e.workers = workers
	e.includeDisabled = includeDisabled
	e.onlyDisabled = onlyDisabled
	if !req.Incremental {
		e.results = nil
		e.bumpResultsLocked()
	}
	e.total = 0
	e.probeDone = 0
	e.applyDone = 0
	e.applyTotal = 0
	e.applyCurrent = ""
	e.applyFailures = nil
	e.startedAt = time.Now()
	e.finishedAt = time.Time{}
	e.runID++
	runID := e.runID
	incremental := req.Incremental
	e.persistLocked()
	e.mu.Unlock()

	e.runWG.Add(1)
	go func() {
		defer e.runWG.Done()
		e.run(runID, workers, includeDisabled, onlyDisabled, incremental)
	}()
	return nil
}

func (e *inspectionEngine) stop() {
	e.mu.Lock()
	e.stopped = true
	e.mu.Unlock()
}

func (e *inspectionEngine) shutdown() {
	e.mu.Lock()
	e.stopped = true
	e.runID++
	e.mu.Unlock()
	e.runWG.Wait()
}

func (e *inspectionEngine) isStopped(runID uint64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopped || e.runID != runID
}

func (e *inspectionEngine) appendResult(runID uint64, result accountResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runID != runID || e.stopped {
		return
	}
	e.results = append(e.results, result)
	e.probeDone++
	e.bumpResultsLocked()
	// Periodic flush so a crash mid-run still keeps partial progress.
	if e.probeDone%10 == 0 {
		e.persistLocked()
	}
}

func (e *inspectionEngine) finish(runID uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runID != runID {
		return
	}
	e.running = false
	e.finishedAt = time.Now()
	e.persistLocked()
}

// knownResultKeys builds identity keys from already-inspected rows so incremental
// mode can skip Auth accounts that appear in the last results.
func knownResultKeys(results []accountResult) map[string]struct{} {
	set := make(map[string]struct{}, len(results)*3)
	for _, item := range results {
		for _, key := range accountIdentityKeys(item.AuthIndex, item.FileName, item.Email, item.Name) {
			set[key] = struct{}{}
		}
	}
	return set
}

func accountIdentityKeys(authIndex, fileName, email, name string) []string {
	keys := make([]string, 0, 4)
	if v := strings.TrimSpace(authIndex); v != "" {
		keys = append(keys, "ai:"+v)
	}
	if v := strings.ToLower(strings.TrimSpace(fileName)); v != "" {
		keys = append(keys, "fn:"+v)
	}
	if v := strings.ToLower(strings.TrimSpace(email)); v != "" {
		keys = append(keys, "em:"+v)
	}
	if v := strings.ToLower(strings.TrimSpace(name)); v != "" {
		keys = append(keys, "nm:"+v)
	}
	return keys
}

func entryIsKnown(known map[string]struct{}, file pluginapi.HostAuthFileEntry) bool {
	for _, key := range accountIdentityKeys(file.AuthIndex, file.Name, file.Email, firstNonEmpty(file.Email, file.Label, file.Name, file.AuthIndex, file.ID)) {
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

func (e *inspectionEngine) run(runID uint64, workers int, includeDisabled, onlyDisabled, incremental bool) {
	defer e.finish(runID)

	list, errList := callHostAuthList()
	if errList != nil {
		e.appendResult(runID, accountResult{
			Name:           "system",
			Classification: "probe_error",
			Action:         "keep",
			Reason:         "列出账号失败: " + errList.Error(),
		})
		return
	}

	var known map[string]struct{}
	if incremental {
		e.mu.Lock()
		known = knownResultKeys(e.results)
		e.mu.Unlock()
	}

	var targets []pluginapi.HostAuthFileEntry
	if incremental {
		targets = filterNewAuthEntries(list.Files, known, includeDisabled, onlyDisabled)
	} else {
		targets = make([]pluginapi.HostAuthFileEntry, 0)
		for _, file := range list.Files {
			if shouldInspectEntry(file.Provider, file.Name, file.Type, file.Disabled, file.Status, includeDisabled, onlyDisabled) {
				targets = append(targets, file)
			}
		}
		sort.Slice(targets, func(i, j int) bool {
			return strings.ToLower(targets[i].Name) < strings.ToLower(targets[j].Name)
		})
	}

	e.mu.Lock()
	if e.runID == runID {
		e.total = len(targets)
	}
	e.mu.Unlock()

	if len(targets) == 0 {
		return
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, file := range targets {
		if e.isStopped(runID) {
			break
		}
		file := file
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if e.isStopped(runID) {
				return
			}
			result := inspectAccount(file)
			e.appendResult(runID, result)
		}()
	}
	wg.Wait()
}

func inspectAccount(file pluginapi.HostAuthFileEntry) accountResult {
	name := firstNonEmpty(file.Email, file.Label, file.Name, file.AuthIndex, file.ID)
	base := accountResult{
		AuthIndex: file.AuthIndex,
		Name:      name,
		FileName:  file.Name,
		Email:     file.Email,
		Disabled:  file.Disabled || isDisabledEntry(file.Disabled, file.Status),
	}
	if strings.TrimSpace(file.AuthIndex) == "" {
		base.Classification = "probe_error"
		base.Action = "keep"
		base.Reason = "缺少 auth_index"
		return base
	}

	modelsResp, errModels := callHostAPICall(file.AuthIndex, http.MethodGet, "https://cli-chat-proxy.grok.com/v1/models", nil, false)
	model := "grok-4.5"
	if errModels == nil && modelsResp.StatusCode >= 200 && modelsResp.StatusCode < 300 {
		model = pickModel(modelsResp.Body)
	}
	base.Model = model

	chatBody := fmt.Sprintf(`{"model":%q,"input":"ping","stream":false}`, model)
	chatResp, errChat := callHostAPICall(file.AuthIndex, http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", []byte(chatBody), true)
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

	status := chatResp.StatusCode
	parsed := extractError(chatResp.Body)
	if status == http.StatusForbidden || status == http.StatusUnauthorized || status == http.StatusTooManyRequests || status == http.StatusPaymentRequired {
		// fallback to chat completions
		fallbackBody := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"stream":false}`, model)
		fallbackResp, errFallback := callHostAPICall(file.AuthIndex, http.MethodPost, "https://cli-chat-proxy.grok.com/v1/chat/completions", []byte(fallbackBody), true)
		if errFallback == nil {
			chatResp = fallbackResp
			status = fallbackResp.StatusCode
			parsed = extractError(fallbackResp.Body)
		}
	}

	classified := classifyProbe(classifyInput{
		ChatStatus: status,
		ChatCode:   parsed.Code,
		ChatError:  parsed.Message,
		Disabled:   base.Disabled,
	})
	base.HTTPStatus = status
	base.ErrorCode = parsed.Code
	base.ErrorMessage = parsed.Message
	base.Classification = classified.Classification
	base.Action = classified.Action
	base.Reason = classified.Reason
	return base
}

type apiCallResponse struct {
	StatusCode int                 `json:"status_code"`
	Header     map[string][]string `json:"header"`
	Body       string              `json:"body"`
}

const xaiInspectionClientVersion = "0.2.93"

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

func callHostAPICall(authIndex, method, rawURL string, body []byte, jsonBody bool) (apiCallResponse, error) {
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

var (
	cpaManagementBaseURL = "http://127.0.0.1:8317"
	cpaManagementClient  = &http.Client{Timeout: 8 * time.Second}
	cpaManagementDo      = func(req *http.Request) (*http.Response, error) {
		return cpaManagementClient.Do(req)
	}
)

func cpaManagementPassword() string {
	return firstNonEmpty(os.Getenv("MANAGEMENT_PASSWORD"), os.Getenv("CPA_MANAGEMENT_KEY"))
}

func extractBearerToken(headers http.Header) string {
	if headers == nil {
		return ""
	}
	// http.Header.Get is case-insensitive for canonical keys.
	auth := strings.TrimSpace(headers.Get("Authorization"))
	if auth == "" {
		// JSON-decoded headers from the host may preserve non-canonical keys.
		for key, values := range headers {
			if strings.EqualFold(strings.TrimSpace(key), "Authorization") && len(values) > 0 {
				auth = strings.TrimSpace(values[0])
				break
			}
		}
	}
	if auth == "" {
		return ""
	}
	const prefix = "bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return auth
}

func resolveManagementPassword(headers http.Header) string {
	if headers == nil {
		return strings.TrimSpace(cpaManagementPassword())
	}
	if token := extractBearerToken(headers); token != "" {
		return token
	}
	if token := strings.TrimSpace(headers.Get("X-Management-Key")); token != "" {
		return token
	}
	for key, values := range headers {
		if strings.EqualFold(strings.TrimSpace(key), "X-Management-Key") && len(values) > 0 {
			if token := strings.TrimSpace(values[0]); token != "" {
				return token
			}
		}
	}
	return strings.TrimSpace(cpaManagementPassword())
}

func resolveManagementBaseURL(headers http.Header) string {
	_ = headers
	// Prefer explicit env. Never derive the management port from the browser Host
	// header: external reverse proxies (e.g. :1109) are not the CPA listen port.
	if value := firstNonEmpty(os.Getenv("CPA_BASE_URL"), os.Getenv("CPA_MANAGEMENT_BASE_URL")); value != "" {
		return strings.TrimRight(strings.TrimSpace(value), "/")
	}
	if port := strings.TrimSpace(firstNonEmpty(os.Getenv("PORT"), os.Getenv("CPA_PORT"))); port != "" {
		return "http://127.0.0.1:" + port
	}
	return strings.TrimRight(cpaManagementBaseURL, "/")
}

func callCPAManagement(method, path string, body []byte) (int, []byte, error) {
	return callCPAManagementWithAuth(method, path, body, "", nil)
}

func callCPAManagementWithAuth(method, path string, body []byte, password string, headers http.Header) (int, []byte, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		password = resolveManagementPassword(headers)
	}
	if password == "" {
		return 0, nil, fmt.Errorf("CPA management password is unavailable (set MANAGEMENT_PASSWORD on CPA process)")
	}
	baseURL := resolveManagementBaseURL(headers)
	req, errRequest := http.NewRequest(method, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if errRequest != nil {
		return 0, nil, errRequest
	}
	req.Header.Set("Authorization", "Bearer "+password)
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, errDo := cpaManagementDo(req)
	if errDo != nil {
		return 0, nil, errDo
	}
	defer resp.Body.Close()
	raw, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return resp.StatusCode, nil, errRead
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, raw, fmt.Errorf("CPA management API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return resp.StatusCode, raw, nil
}

// findAuthFromResults resolves an auth identity from the in-memory inspection
// list without calling host.auth.list (which is O(n) over all CPA accounts).
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
	list, errList := callHostAuthList()
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

// setAuthDisabled toggles CPA auth via Management API PATCH /auth-files/status.
// host.auth.save alone is NOT enough: CLIProxyAPI buildAuthFromFileData does not
// promote JSON "disabled" onto Auth.Disabled, so the main UI stays enabled.
// Must run outside management.handle (background goroutine) to avoid re-entry deadlock.
//
// After a successful PATCH we trust CPA and update local results only — we do not
// re-list all auth files (that was the main reason single-row ops felt slow).
// persist=false is used by bulk apply so 1000 disk flushes do not dominate runtime.
func setAuthDisabled(name string, disabled bool, password string, headers http.Header, persist bool) error {
	target, errTarget := findAuthFile(name)
	if errTarget != nil {
		return errTarget
	}
	fileName := firstNonEmpty(target.Name, target.ID)
	if strings.TrimSpace(fileName) == "" {
		return fmt.Errorf("auth file name missing for %s", name)
	}
	body, errMarshal := json.Marshal(map[string]any{
		"name":     fileName,
		"disabled": disabled,
	})
	if errMarshal != nil {
		return errMarshal
	}
	if _, _, errPatch := callCPAManagementWithAuth(http.MethodPatch, "/v0/management/auth-files/status", body, password, headers); errPatch != nil {
		return errPatch
	}
	engine.mu.Lock()
	for i := range engine.results {
		item := &engine.results[i]
		if resultMatchesTarget(*item, target, name) {
			item.Disabled = disabled
			if disabled && item.Action == "disable" {
				item.Action = "keep"
			}
			if !disabled && item.Action == "enable" {
				item.Action = "keep"
			}
			if disabled && item.Classification == "healthy" {
				item.Action = "enable"
			}
		}
	}
	engine.bumpResultsLocked()
	if persist {
		engine.persistLocked()
	}
	engine.mu.Unlock()
	return nil
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

// removeResultLocked drops matching rows from the in-memory list. Caller must hold e.mu.
func (e *inspectionEngine) removeResultLocked(target *pluginapi.HostAuthFileEntry, name string) {
	kept := make([]accountResult, 0, len(e.results))
	for _, item := range e.results {
		if resultMatchesTarget(item, target, name) {
			continue
		}
		kept = append(kept, item)
	}
	e.results = kept
	e.bumpResultsLocked()
	if !e.running {
		e.total = len(e.results)
	}
}

// deleteAuthFile must only be called from a background goroutine after management.handle
// has returned, so it does not deadlock on the management lock.
// It deletes the CPA Auth credential file AND removes the row from local JSON results.
// password/headers come from the page Management Key (or env fallbacks) so third-party
// installs work without MANAGEMENT_PASSWORD on the process.
func deleteAuthFile(name string, password string, headers http.Header, persist bool) error {
	target, errTarget := findAuthFile(name)
	if errTarget != nil {
		// Idempotent: already gone counts as success for delete recommendations.
		if strings.Contains(errTarget.Error(), "auth not found") {
			engine.mu.Lock()
			engine.removeResultLocked(nil, name)
			if persist {
				engine.persistLocked()
			}
			engine.mu.Unlock()
			return nil
		}
		return errTarget
	}
	fileName := firstNonEmpty(target.Name)
	if fileName == "" {
		return fmt.Errorf("auth file name missing for %s", name)
	}
	path := "/v0/management/auth-files?name=" + url.QueryEscape(fileName)
	if _, _, errDelete := callCPAManagementWithAuth(http.MethodDelete, path, nil, password, headers); errDelete != nil {
		// Concurrent deletes / already-removed files often surface as 404 — treat as success.
		msg := errDelete.Error()
		if strings.Contains(msg, "404") || strings.Contains(strings.ToLower(msg), "not found") {
			engine.mu.Lock()
			engine.removeResultLocked(target, name)
			if persist {
				engine.persistLocked()
			}
			engine.mu.Unlock()
			return nil
		}
		return errDelete
	}
	// Trust a successful DELETE; do not re-list all CPA auth files to verify.
	engine.mu.Lock()
	engine.removeResultLocked(target, name)
	if persist {
		engine.persistLocked()
	}
	engine.mu.Unlock()
	return nil
}

// resolveDeleteFileName picks the CPA auth file name for management DELETE.
func resolveDeleteFileName(item accountResult) string {
	return firstNonEmpty(item.FileName, item.Name, item.AuthIndex, item.Email)
}

// deleteAuthFilesBatch deletes many auth files in one CPA Management API call.
// Host supports: DELETE /v0/management/auth-files with body {"names":[...]} or multi ?name=.
// Returns per-item failure messages (empty means all ok).
func deleteAuthFilesBatch(items []accountResult, password string, headers http.Header, persist bool) []string {
	if len(items) == 0 {
		return nil
	}
	type mapped struct {
		item     accountResult
		fileName string
	}
	mappedItems := make([]mapped, 0, len(items))
	names := make([]string, 0, len(items))
	failures := make([]string, 0)
	seenName := map[string]struct{}{}
	for _, item := range items {
		fileName := resolveDeleteFileName(item)
		if fileName == "" {
			failures = append(failures, item.Name+": auth file name missing")
			continue
		}
		// Skip duplicate physical names in the same batch (same file, multiple result rows).
		if _, ok := seenName[fileName]; ok {
			// Still drop matching local rows for this identity.
			engine.mu.Lock()
			engine.removeResultLocked(nil, firstNonEmpty(item.AuthIndex, item.FileName, item.Name, item.Email))
			engine.mu.Unlock()
			continue
		}
		seenName[fileName] = struct{}{}
		mappedItems = append(mappedItems, mapped{item: item, fileName: fileName})
		names = append(names, fileName)
	}
	if len(names) == 0 {
		if persist {
			engine.persist()
		}
		return failures
	}

	body, errMarshal := json.Marshal(map[string]any{"names": names})
	if errMarshal != nil {
		for _, m := range mappedItems {
			failures = append(failures, m.item.Name+": "+errMarshal.Error())
		}
		return failures
	}

	status, raw, errDelete := callCPAManagementWithAuth(http.MethodDelete, "/v0/management/auth-files", body, password, headers)
	if errDelete != nil {
		// Whole batch failed (network / hard error). Mark all remaining names failed.
		msg := errDelete.Error()
		// If everything was already gone, treat as success.
		if strings.Contains(msg, "404") || strings.Contains(strings.ToLower(msg), "not found") {
			engine.mu.Lock()
			for _, m := range mappedItems {
				engine.removeResultLocked(nil, firstNonEmpty(m.item.AuthIndex, m.fileName, m.item.Name, m.item.Email))
			}
			if persist {
				engine.persistLocked()
			}
			engine.mu.Unlock()
			return failures
		}
		for _, m := range mappedItems {
			failures = append(failures, m.item.Name+": "+msg)
		}
		return failures
	}

	// Parse optional partial failure payload (HTTP 207 Multi-Status).
	failedNames := map[string]string{}
	if status == http.StatusMultiStatus || len(raw) > 0 {
		var payload struct {
			Status  string `json:"status"`
			Failed  []struct {
				Name  string `json:"name"`
				Error string `json:"error"`
			} `json:"failed"`
			Files []string `json:"files"`
		}
		if err := json.Unmarshal(raw, &payload); err == nil {
			for _, f := range payload.Failed {
				name := strings.TrimSpace(f.Name)
				if name == "" {
					continue
				}
				errText := strings.TrimSpace(f.Error)
				if errText == "" {
					errText = "delete failed"
				}
				failedNames[name] = errText
			}
		}
	}

	engine.mu.Lock()
	for _, m := range mappedItems {
		if errText, ok := failedNames[m.fileName]; ok {
			failures = append(failures, m.item.Name+": "+errText)
			continue
		}
		engine.removeResultLocked(nil, firstNonEmpty(m.item.AuthIndex, m.fileName, m.item.Name, m.item.Email))
	}
	if persist {
		engine.persistLocked()
	}
	engine.mu.Unlock()
	_ = status
	return failures
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

func normalizeForceAction(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "disable", "enable", "delete":
		return value, nil
	default:
		return "", fmt.Errorf("force_action must be disable, enable, or delete")
	}
}

func itemSelected(item accountResult, indexSet, classSet map[string]struct{}) bool {
	if len(classSet) > 0 {
		if _, ok := classSet[item.Classification]; !ok {
			return false
		}
	}
	if len(indexSet) == 0 {
		return true
	}
	if _, ok := indexSet[item.AuthIndex]; ok {
		return true
	}
	if _, ok := indexSet[item.Name]; ok {
		return true
	}
	if _, ok := indexSet[item.FileName]; ok {
		return true
	}
	if _, ok := indexSet[item.Email]; ok {
		return true
	}
	return false
}

func (e *inspectionEngine) collectCandidates(req applyRequest) ([]accountResult, error) {
	force, errForce := normalizeForceAction(req.ForceAction)
	if errForce != nil {
		return nil, errForce
	}
	indexSet := stringSet(req.AuthIndexes)
	actionSet := stringSet(req.Actions)
	classSet := stringSet(req.Classifications)
	// Filter-based bulk ops must name targets (or classification) explicitly.
	if force != "" && len(indexSet) == 0 && len(classSet) == 0 {
		return nil, fmt.Errorf("force_action requires auth_indexes or classifications")
	}

	candidates := make([]accountResult, 0)
	for _, item := range e.results {
		if !itemSelected(item, indexSet, classSet) {
			continue
		}
		if force != "" {
			copied := item
			copied.Action = force
			candidates = append(candidates, copied)
			continue
		}
		// Recommended-only mode (执行建议操作)
		if item.Action != "disable" && item.Action != "enable" && item.Action != "delete" {
			continue
		}
		if len(actionSet) > 0 {
			if _, ok := actionSet[item.Action]; !ok {
				continue
			}
		}
		candidates = append(candidates, item)
	}
	return candidates, nil
}

// startApply runs recommended or forced bulk actions asynchronously.
// password/headers are captured for background delete calls (page Management Key).
func cloneHTTPHeader(src http.Header) http.Header {
	if src == nil {
		return nil
	}
	dst := make(http.Header, len(src))
	for k, vals := range src {
		dst[k] = append([]string(nil), vals...)
	}
	return dst
}

func (e *inspectionEngine) startApply(req applyRequest, password string, headers http.Header) error {
	e.mu.Lock()
	if e.running || e.applying || e.actionInFlight > 0 {
		e.mu.Unlock()
		return fmt.Errorf("busy")
	}
	candidates, errCollect := e.collectCandidates(req)
	if errCollect != nil {
		e.mu.Unlock()
		return errCollect
	}
	if len(candidates) == 0 {
		e.mu.Unlock()
		if strings.TrimSpace(req.ForceAction) != "" {
			return fmt.Errorf("no accounts matched current selection")
		}
		return fmt.Errorf("no recommended actions")
	}
	e.applying = true
	e.applyDone = 0
	e.applyTotal = len(candidates)
	e.applyCurrent = ""
	e.applyFailures = nil
	// Capture auth material for the background goroutine (request may free headers after return).
	password = strings.TrimSpace(password)
	headers = cloneHTTPHeader(headers)
	e.mu.Unlock()

	e.runWG.Add(1)
	go func() {
		defer e.runWG.Done()
		e.runApply(candidates, password, headers)
	}()
	return nil
}

// startAction runs a single enable/disable/delete asynchronously.
// Unlike bulk apply, it does not set applying=true so other list rows stay clickable,
// but actionInFlight blocks full inspect / bulk apply to avoid result races.
func (e *inspectionEngine) startAction(req actionRequest, password string, headers http.Header) error {
	name := firstNonEmpty(req.Name, req.AuthIndex)
	if name == "" {
		return fmt.Errorf("name or auth_index required")
	}
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("busy: inspection running")
	}
	if e.applying {
		e.mu.Unlock()
		return fmt.Errorf("busy: bulk apply in progress")
	}
	e.actionInFlight++
	password = strings.TrimSpace(password)
	headers = cloneHTTPHeader(headers)
	e.mu.Unlock()

	e.runWG.Add(1)
	go func() {
		defer e.runWG.Done()
		// Yield so management.handle can finish before we re-enter Management HTTP
		// (avoids re-entry deadlock while keeping the UI path snappy).
		time.Sleep(5 * time.Millisecond)
		var errAction error
		if req.Delete {
			errAction = deleteAuthFile(name, password, headers, true)
		} else {
			errAction = setAuthDisabled(name, req.Disabled, password, headers, true)
		}
		e.mu.Lock()
		e.actionInFlight--
		if e.actionInFlight < 0 {
			e.actionInFlight = 0
		}
		if errAction != nil {
			// Keep a short recent failure list; do not wipe other concurrent row failures.
			msg := name + ": " + errAction.Error()
			e.applyFailures = append([]string{msg}, e.applyFailures...)
			if len(e.applyFailures) > 20 {
				e.applyFailures = e.applyFailures[:20]
			}
			e.persistLocked()
		}
		// Success path already persisted inside setAuthDisabled/deleteAuthFile.
		e.mu.Unlock()
	}()
	return nil
}

func (e *inspectionEngine) runApply(candidates []accountResult, password string, headers http.Header) {
	defer func() {
		e.mu.Lock()
		e.applying = false
		e.applyCurrent = ""
		e.persistLocked()
		e.mu.Unlock()
	}()

	// Split deletes (host batch API) from enable/disable (host single-item only).
	deletes := make([]accountResult, 0)
	others := make([]accountResult, 0)
	for _, item := range candidates {
		if item.Action == "delete" {
			deletes = append(deletes, item)
		} else {
			others = append(others, item)
		}
	}

	// --- Bulk delete via CPA multi-name DELETE (chunks of deleteBatchSize) ---
	for i := 0; i < len(deletes); i += deleteBatchSize {
		end := i + deleteBatchSize
		if end > len(deletes) {
			end = len(deletes)
		}
		chunk := deletes[i:end]
		e.mu.Lock()
		e.applyCurrent = fmt.Sprintf("delete batch %d-%d/%d", i+1, end, len(deletes))
		e.mu.Unlock()

		batchFails := deleteAuthFilesBatch(chunk, password, headers, false)
		e.mu.Lock()
		if len(batchFails) > 0 {
			e.applyFailures = append(e.applyFailures, batchFails...)
		}
		e.applyDone += len(chunk)
		if e.applyDone%applyPersistEvery == 0 || end == len(deletes) {
			e.persistLocked()
		}
		e.mu.Unlock()
	}

	// --- Enable/disable: no host batch API → concurrent single PATCH ---
	if len(others) == 0 {
		return
	}
	workers := defaultApplyWorkers
	if workers > maxApplyWorkers {
		workers = maxApplyWorkers
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(others) {
		workers = len(others)
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, item := range others {
		item := item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			e.mu.Lock()
			e.applyCurrent = item.Action + " " + item.Name
			e.mu.Unlock()

			// Prefer physical auth file name so CPA Auth dir entry is deleted correctly.
			targetName := firstNonEmpty(item.FileName, item.AuthIndex, item.Name, item.Email)
			var errAction error
			switch item.Action {
			case "disable":
				errAction = setAuthDisabled(targetName, true, password, headers, false)
			case "enable":
				errAction = setAuthDisabled(targetName, false, password, headers, false)
			default:
				errAction = fmt.Errorf("unsupported action %q", item.Action)
			}

			e.mu.Lock()
			if errAction != nil {
				e.applyFailures = append(e.applyFailures, item.Name+": "+errAction.Error())
			}
			e.applyDone++
			done := e.applyDone
			// Occasional flush during bulk — full persist only every N items + final defer.
			if done%applyPersistEvery == 0 {
				e.persistLocked()
			}
			e.mu.Unlock()
		}()
	}
	wg.Wait()
}
