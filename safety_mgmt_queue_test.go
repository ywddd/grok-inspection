package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

// ---------- 1) 403 permission vs content safety ----------

func TestDetectBanRejectsContentSafety403(t *testing.T) {
	cfg := defaultPluginConfig()
	now := time.Now()
	bodies := []string{
		`{"code":"permission-denied","error":"Content violates usage guidelines"}`,
		`{"code":"SAFETY_CHECK_FAILED","error":"blocked by content safety"}`,
		`{"code":"moderation","error":"policy violation: unsafe content"}`,
		`{"code":"permission-denied","error":"Request blocked by safety policy"}`,
	}
	for _, body := range bodies {
		record := pluginapi.UsageRecord{
			Provider: "xai",
			AuthID:   "safe-auth",
			Failed:   true,
			Failure:  pluginapi.UsageFailure{StatusCode: 403, Body: body},
		}
		if entry, ok := detectBan(record, cfg, now); ok {
			t.Fatalf("content safety must not auto-ban: body=%s entry=%#v", body, entry)
		}
	}
}

func TestDetectBanRequiresAccountLevelPermissionEvidence(t *testing.T) {
	cfg := defaultPluginConfig()
	now := time.Now()
	// Bare code=permission-denied without account-level wording must not ban.
	record := pluginapi.UsageRecord{
		Provider: "xai",
		AuthID:   "bare-403",
		Failed:   true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 403,
			Body:       `{"code":"permission-denied","error":"forbidden"}`,
		},
	}
	if _, ok := detectBan(record, cfg, now); ok {
		t.Fatal("bare permission-denied without account evidence must not ban")
	}
	// Real Grok account-level denial still bans.
	record.Failure.Body = realGrok403Body
	if _, ok := detectBan(record, cfg, now); !ok {
		t.Fatal("real Grok chat endpoint denial must still ban")
	}
}

func TestDetectBanRejectsNonAccount403Codes(t *testing.T) {
	cfg := defaultPluginConfig()
	now := time.Now()
	for _, body := range []string{
		`{"code":"INSUFFICIENT_BALANCE","error":"not enough credits"}`,
		`{"code":"30001","error":"balance low"}`,
	} {
		record := pluginapi.UsageRecord{
			Provider: "xai", AuthID: "bal", Failed: true,
			Failure: pluginapi.UsageFailure{StatusCode: 403, Body: body},
		}
		if _, ok := detectBan(record, cfg, now); ok {
			t.Fatalf("non-permission 403 must not ban: %s", body)
		}
	}
}

func TestClassifyContentSafetyIsProbeErrorKeep(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang: LangZH, ChatStatus: 403,
		ChatCode: "permission-denied", ChatError: "Content violates usage guidelines",
	})
	if got.Classification == "permission_denied" || got.Action == "disable" {
		t.Fatalf("safety block must not recommend disable: %+v", got)
	}
	if got.Classification != "probe_error" && got.Classification != "unknown" {
		t.Fatalf("safety block classification = %q", got.Classification)
	}
}

func TestClassifyBareCloudflare403IsNotPermissionDenied(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang: LangEN, ChatStatus: 403,
		ChatCode: "", ChatError: "Just a moment... Cloudflare",
	})
	if got.Classification == "permission_denied" || got.Action == "disable" {
		t.Fatalf("WAF/Cloudflare 403 must not be permission_denied: %+v", got)
	}
}

func TestClassifyAccountLevelPermissionDeniedStillWorks(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang: LangZH, ChatStatus: 403,
		ChatCode:  "permission-denied",
		ChatError: "Access to the chat endpoint is denied. Please ensure you're using the correct credentials.",
	})
	if got.Classification != "permission_denied" || got.Action != "disable" {
		t.Fatalf("account-level 403: %+v", got)
	}
}

// ---------- 2) Management credential cache ----------

func TestManagementCredentialPrefersExplicitThenCacheThenEnv(t *testing.T) {
	clearManagementCredentialCacheForTest()
	t.Cleanup(clearManagementCredentialCacheForTest)
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	oldKey := os.Getenv("CPA_MANAGEMENT_KEY")
	_ = os.Unsetenv("MANAGEMENT_PASSWORD")
	_ = os.Unsetenv("CPA_MANAGEMENT_KEY")
	t.Cleanup(func() {
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
		_ = os.Setenv("CPA_MANAGEMENT_KEY", oldKey)
	})

	if got := resolveManagementPassword(nil); got != "" {
		t.Fatalf("expected empty without cache/env, got %q", got)
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer page-key-1")
	if got := resolveManagementPassword(headers); got != "page-key-1" {
		t.Fatalf("explicit header key = %q", got)
	}
	// After authenticated request, cache should serve realtime disable path.
	if got := cpaManagementPasswordOrCached(); got != "page-key-1" {
		t.Fatalf("cached key = %q", got)
	}
	_ = os.Setenv("MANAGEMENT_PASSWORD", "env-key")
	// Explicit/cached still wins over env for resolve with empty headers when cache set.
	if got := resolveManagementPassword(nil); got != "page-key-1" {
		t.Fatalf("cache should beat env, got %q", got)
	}
	clearManagementCredentialCacheForTest()
	if got := resolveManagementPassword(nil); got != "env-key" {
		t.Fatalf("env fallback = %q", got)
	}
}

func TestRealtimeDisableUsesCachedManagementKey(t *testing.T) {
	clearManagementCredentialCacheForTest()
	t.Cleanup(clearManagementCredentialCacheForTest)
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	_ = os.Unsetenv("MANAGEMENT_PASSWORD")
	_ = os.Unsetenv("CPA_MANAGEMENT_KEY")
	t.Cleanup(func() { _ = os.Setenv("MANAGEMENT_PASSWORD", oldPass) })

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	oldBase, oldDo := getCPAManagementBaseURL(), getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() { cpaManagementBaseURL = oldBase; cpaManagementDo = oldDo })

	rememberManagementCredential("ui-only-key")
	if err := disableAuthInCPA("acct-1"); err != nil {
		t.Fatalf("disableAuthInCPA: %v", err)
	}
	if gotAuth != "Bearer ui-only-key" {
		t.Fatalf("Authorization = %q, want Bearer ui-only-key", gotAuth)
	}
}

// ---------- 3) Sync status visibility ----------

func TestBanStatusExposesSyncErrorAndClearsOnSuccess(t *testing.T) {
	isolateActiveStore(t)
	activeStore.Set(banEntry{
		AuthID: "sync-1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: time.Now(), ResetAt: time.Now().Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: false,
		CpaSyncError: "CPA management API returned HTTP 500: boom",
	})
	st := banStatus()
	if st["unsynced_count"] != 1 {
		t.Fatalf("unsynced_count = %v", st["unsynced_count"])
	}
	bans, _ := st["bans"].([]map[string]any)
	if len(bans) != 1 || bans[0]["cpa_synced"] != false {
		t.Fatalf("bans = %#v", bans)
	}
	if errMsg, _ := bans[0]["cpa_sync_error"].(string); !strings.Contains(errMsg, "HTTP 500") {
		t.Fatalf("cpa_sync_error = %v", bans[0]["cpa_sync_error"])
	}
	if strings.Contains(fmt.Sprint(bans[0]["cpa_sync_error"]), "Bearer") {
		t.Fatal("sync error must not leak credentials")
	}
	activeStore.UpdateCpaSynced("sync-1", true)
	entry, _ := activeStore.Get("sync-1")
	if !entry.CpaSynced || entry.CpaSyncError != "" {
		t.Fatalf("after success: %#v", entry)
	}
}

func TestUIShowsUnsyncedBanStatus(t *testing.T) {
	page := string(renderUIPage(pluginName))
	for _, needle := range []string{
		"cpa_synced", "unsynced_count", "ban_unsynced", "ban_sync_error",
		"未同步", "Unsynced",
	} {
		if !strings.Contains(page, needle) {
			t.Fatalf("UI missing %q", needle)
		}
	}
}

// ---------- 4) Usage callback non-blocking queue ----------

func TestHandleUsageDoesNotBlockOnManagementPatch(t *testing.T) {
	isolateActiveStore(t)
	// Pause workers so no background PATCH races cleanup / -race.
	// Spec under test: usage path itself never blocks on Management I/O.
	pauseBanDisposeWorkersForTest(t)
	resetBanDisposeQueueForTest(t)
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.Enabled = true
	cfg.PersistState = false
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	block := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	oldBase, oldDo := getCPAManagementBaseURL(), getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		close(block)
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")

	record := pluginapi.UsageRecord{
		Provider: "xai", AuthID: "slow-auth", Failed: true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 429,
			Body:       `{"code":"subscription:free-usage-exhausted","error":"used all"}`,
			// headers for reset optional
		},
	}
	// Attach headers if needed via zero value ok with fallback.
	start := time.Now()
	entry, err := handleUsageRecord(record, cfg, time.Now())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("handleUsageRecord: %v", err)
	}
	if entry.AuthID == "" {
		t.Fatal("expected local ban entry")
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("usage handle blocked %s on management PATCH", elapsed)
	}
	// Local ban exists and is unsynced until worker finishes.
	got, ok := activeStore.Get("slow-auth")
	if !ok || got.CpaSynced {
		t.Fatalf("expected unsynced local ban: ok=%v %#v", ok, got)
	}
}

func TestBanDisposeQueueCapacityAndDedupDeterministic(t *testing.T) {
	// Capacity semantics (explicit):
	//   - Capacity counts QUEUED work only (pending map / queued counter).
	//   - In-flight work (already taken by a worker) does NOT occupy a slot.
	// Determinism: local queue with NO workers so nothing dequeues.
	const capN = 8
	q := newBanDisposeQueue(capN, 1)

	accepted := 0
	for i := 0; i < capN; i++ {
		if !q.enqueue(fmt.Sprintf("a-%d", i), 1) {
			t.Fatalf("enqueue %d rejected before capacity", i)
		}
		accepted++
	}
	if accepted != capN {
		t.Fatalf("accepted=%d want capacity=%d (queued-only capacity)", accepted, capN)
	}
	if q.queuedCount() != capN || q.pendingCount() != capN {
		t.Fatalf("queued=%d pending=%d want %d", q.queuedCount(), q.pendingCount(), capN)
	}
	if q.enqueue("overflow-new", 1) {
		t.Fatal("expected reject when queued capacity full")
	}
	if q.queuedCount() != capN {
		t.Fatalf("overflow must not change queued count: %d", q.queuedCount())
	}
	beforeQ, beforeP := q.queuedCount(), q.pendingCount()
	if !q.enqueue("a-0", 99) {
		t.Fatal("dedupe enqueue of existing authID should succeed")
	}
	if q.queuedCount() != beforeQ || q.pendingCount() != beforeP {
		t.Fatalf("dedupe grew queue: q %d->%d p %d->%d", beforeQ, q.queuedCount(), beforeP, q.pendingCount())
	}
	q.mu.Lock()
	if q.pending["a-0"] != 99 {
		t.Fatalf("dedupe did not bump revision: %d", q.pending["a-0"])
	}
	q.mu.Unlock()
	done := make(chan bool, 1)
	go func() {
		done <- q.enqueue("overflow-async", 1)
	}()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("overflow enqueue must return false when full")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("enqueue blocked when full")
	}
}

func TestBanDisposeQueueDedupesByAuthIDAndDoesNotBlockWhenFull(t *testing.T) {
	// Global queue + paused workers: capacity is still queued-only.
	pauseBanDisposeWorkersForTest(t)
	resetBanDisposeQueueForTest(t)

	if banDisposeQueuedCountForTest() != 0 || banDisposePendingCountForTest() != 0 {
		t.Fatalf("queue not empty: queued=%d pending=%d", banDisposeQueuedCountForTest(), banDisposePendingCountForTest())
	}

	accepted := 0
	for i := 0; i < banDisposeQueueCapacity; i++ {
		if !enqueueBanDispose(fmt.Sprintf("a-%d", i), 1) {
			t.Fatalf("enqueue %d rejected before capacity", i)
		}
		accepted++
	}
	if accepted != banDisposeQueueCapacity {
		t.Fatalf("accepted=%d want capacity=%d", accepted, banDisposeQueueCapacity)
	}
	if banDisposeQueuedCountForTest() != banDisposeQueueCapacity {
		t.Fatalf("queued=%d want %d", banDisposeQueuedCountForTest(), banDisposeQueueCapacity)
	}
	if enqueueBanDispose("overflow-new", 1) {
		t.Fatal("expected reject when queued capacity full")
	}
	beforeQ := banDisposeQueuedCountForTest()
	beforeP := banDisposePendingCountForTest()
	if !enqueueBanDispose("a-0", 2) {
		t.Fatal("dedupe enqueue of existing authID should succeed")
	}
	if banDisposeQueuedCountForTest() != beforeQ || banDisposePendingCountForTest() != beforeP {
		t.Fatalf("dedupe grew queue: q %d->%d p %d->%d", beforeQ, banDisposeQueuedCountForTest(), beforeP, banDisposePendingCountForTest())
	}
	done := make(chan bool, 1)
	go func() {
		done <- enqueueBanDispose("overflow-async", 1)
	}()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("overflow enqueue must return false when full")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("enqueue blocked when full")
	}
}

func TestHandleUsageQueueFullLeavesUnsyncedRetriable(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	resetBanDisposeQueueForTest(t)
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.Enabled = true
	cfg.PersistState = false
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	// Saturate queued capacity (queued-only; workers held so slots stay occupied).
	markBanDisposeQueueFullForTest(t)
	if banDisposeQueuedCountForTest() != banDisposeQueueCapacity {
		t.Fatalf("queued=%d want %d after mark full", banDisposeQueuedCountForTest(), banDisposeQueueCapacity)
	}
	record := pluginapi.UsageRecord{
		Provider: "xai", AuthID: "full-q-auth", Failed: true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 429,
			Body:       `{"code":"subscription:free-usage-exhausted","error":"used all the included free usage"}`,
		},
	}
	entry, err := handleUsageRecord(record, cfg, time.Now())
	if err != nil {
		t.Fatalf("handleUsageRecord: %v", err)
	}
	if entry.AuthID != "full-q-auth" {
		t.Fatalf("local ban missing: %#v", entry)
	}
	got, ok := activeStore.Get("full-q-auth")
	if !ok || got.CpaSynced {
		t.Fatalf("must stay unsynced when queue full: ok=%v %#v", ok, got)
	}
	if got.CpaSyncError == "" {
		t.Fatal("expected cpa_sync_error when queue full")
	}
	// Retriable via restore path for unsynced entries.
	unsynced := activeStore.UnsyncedCPA()
	found := false
	for _, e := range unsynced {
		if e.AuthID == "full-q-auth" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("unsynced list must include queue-full ban for retry")
	}
}

// ---------- 5) Per-account serialization + revision ----------

func TestBanStoreSetAdvancesRevisionWhenKeepingLongerReset(t *testing.T) {
	store := newBanStore()
	store.Set(testEntry("r1", time.Unix(200, 0)))
	first, _ := store.Get("r1")
	store.Set(testEntry("r1", time.Unix(50, 0))) // shorter window
	second, _ := store.Get("r1")
	if !second.ResetAt.Equal(time.Unix(200, 0)) {
		t.Fatalf("must keep longer ResetAt: %v", second.ResetAt)
	}
	if second.Revision <= first.Revision {
		t.Fatalf("revision must advance: first=%d second=%d", first.Revision, second.Revision)
	}
}

func TestOldDisableDoesNotClobberNewerBanRevision(t *testing.T) {
	isolateActiveStore(t)
	resetBanDisposeQueueForTest(t)
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.Enabled = true
	cfg.PersistState = false
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	var disableCalls atomic.Int32
	var secondDisable atomic.Int32
	blockFirst := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := disableCalls.Add(1)
		if n == 1 {
			<-blockFirst
		} else {
			secondDisable.Add(1)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	oldBase, oldDo := getCPAManagementBaseURL(), getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	var closeOnce sync.Once
	closeFirst := func() { closeOnce.Do(func() { close(blockFirst) }) }
	t.Cleanup(func() {
		closeFirst()
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	// Seed old long ban then start delayed dispose worker path via direct apply.
	activeStore.Set(banEntry{
		AuthID: "race-rev", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: time.Now().Add(-time.Hour), ResetAt: time.Now().Add(2 * time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: false,
	})
	old, _ := activeStore.Get("race-rev")
	// Simulate delayed dispose of old revision while a newer ban lands.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = applyBanDisposeForTest("race-rev", old.Revision)
	}()
	// Wait until first disable is in flight.
	deadline := time.Now().Add(2 * time.Second)
	for disableCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	// Shorter new ban must still bump revision.
	activeStore.Set(banEntry{
		AuthID: "race-rev", Provider: "xai", ErrorCode: permissionDeniedErrorCode,
		BannedAt: time.Now(), ResetAt: time.Now().Add(30 * time.Minute),
		ResetSource: "manual_unban", CpaSynced: false,
	})
	newer, _ := activeStore.Get("race-rev")
	if newer.Revision <= old.Revision {
		t.Fatalf("newer rev not advanced: old=%d new=%d", old.Revision, newer.Revision)
	}
	closeFirst()
	wg.Wait()
	// Old dispose finishing must not mark stale revision as the sole synced state incorrectly.
	got, ok := activeStore.Get("race-rev")
	if !ok {
		t.Fatal("ban disappeared")
	}
	if got.Revision != newer.Revision {
		t.Fatalf("revision clobbered: got=%d want=%d", got.Revision, newer.Revision)
	}
}

func TestUnbanDuringShorterResetBanKeepsNewer(t *testing.T) {
	// Covered pattern: unban in flight, shorter reset Set advances rev, unban must not delete.
	var enableCalls atomic.Int32
	blockEnable := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enableCalls.Add(1)
		<-blockEnable
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	oldBase, oldDo := getCPAManagementBaseURL(), getCPAManagementDo()
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	})
	isolateUnbanJob(t)
	isolateActiveStore(t)
	activeStore.Set(banEntry{
		AuthID: "short-race", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: time.Now().Add(-time.Hour), ResetAt: time.Now().Add(3 * time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	old, _ := activeStore.Get("short-race")
	if err := startUnbanJob([]string{"short-race"}, "", "test-pass"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for enableCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	// Shorter new ban while unban in flight.
	activeStore.Set(banEntry{
		AuthID: "short-race", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: time.Now(), ResetAt: time.Now().Add(10 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: true,
	})
	newer, _ := activeStore.Get("short-race")
	if newer.Revision <= old.Revision {
		close(blockEnable)
		t.Fatalf("shorter reset must advance revision")
	}
	close(blockEnable)
	unbanJob.wg.Wait()
	got, still := activeStore.Get("short-race")
	if !still {
		t.Fatal("unban deleted newer shorter-reset ban")
	}
	if got.Revision != newer.Revision {
		t.Fatalf("rev=%d want=%d", got.Revision, newer.Revision)
	}
}

// ---------- 6) TLS security ----------

func TestManagementTLSInsecureOnlyForLoopbackOrOptIn(t *testing.T) {
	if !managementTLSSkipVerifyForHost("127.0.0.1") {
		t.Fatal("loopback must allow insecure")
	}
	if !managementTLSSkipVerifyForHost("localhost") {
		t.Fatal("localhost must allow insecure")
	}
	if managementTLSSkipVerifyForHost("cpa.example.com") {
		t.Fatal("remote host must verify certs by default")
	}
	t.Setenv("GROK_INSPECTION_INSECURE_REMOTE_TLS", "1")
	if !managementTLSSkipVerifyForHost("cpa.example.com") {
		t.Fatal("opt-in env must allow remote insecure")
	}
	// Ensure production client transport is selective, not globally insecure-only.
	client := newManagementHTTPClient()
	tr, ok := client.Transport.(*http.Transport)
	if ok && tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		// Global true is only acceptable if VerifyConnection/custom dial enforces host policy.
		if !managementTLSConfigIsSelective(client) {
			t.Fatal("management client must not globally InsecureSkipVerify without host selection")
		}
	}
	_ = tls.VersionTLS12
	_ = net.IPv4zero
}

// ---------- 7) Shutdown safety ----------

func TestShutdownWaitsForHostCallsAndEmitsDiagnostics(t *testing.T) {
	// Isolate gate/WG from other tests: drain any leftover slots carefully.
	for hostCallInflight() > 0 {
		select {
		case <-hostCallGate:
			hostCallWG.Done()
		default:
			goto drained
		}
	}
drained:
	acquireHostCall()

	var (
		logMu sync.Mutex
		logs  []string
	)
	oldLog := shutdownWaitLogger
	shutdownWaitLogger = func(msg string, args ...any) {
		logMu.Lock()
		logs = append(logs, msg)
		logMu.Unlock()
	}
	t.Cleanup(func() {
		shutdownWaitLogger = oldLog
		// Best-effort release if still held.
		if hostCallInflight() > 0 {
			releaseHostCall()
		}
		rearmHostCallAdmissionForTest()
	})

	done := make(chan struct{})
	go func() {
		waitHostCallsForShutdown(40 * time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("shutdown wait returned while host call inflight")
	case <-time.After(100 * time.Millisecond):
	}
	releaseHostCall()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown wait did not finish after release")
	}
	logMu.Lock()
	n := len(logs)
	logMu.Unlock()
	if n == 0 {
		t.Fatal("expected diagnostic log while waiting")
	}
}

// ---------- 8) Schedule real persistence ----------

func TestUpdateInspectionScheduleRequiresDiskSuccess(t *testing.T) {
	dir := t.TempDir()
	// Point store to a non-writable schedule path by using a file as directory parent trick:
	// schedule.json path under a missing read-only parent.
	badParent := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(badParent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	setStoreFilePathForTest(filepath.Join(badParent, "results.json"))
	resetStoreIOForTest()
	engine.mu.Lock()
	old := engine.schedule
	engine.schedule = defaultInspectionSchedule()
	prevEnabled := engine.schedule.Enabled
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.schedule = old
		engine.mu.Unlock()
		setStoreFilePathForTest("")
		resetStoreIOForTest()
	})

	enabled := true
	interval := 5
	_, err := updateInspectionSchedule(inspectionScheduleUpdate{
		Enabled: &enabled, IntervalMinutes: &interval,
	})
	if err == nil {
		t.Fatal("expected disk failure")
	}
	engine.mu.Lock()
	cfg := engine.schedule
	engine.mu.Unlock()
	if cfg.Enabled != prevEnabled {
		t.Fatalf("memory schedule must roll back on disk failure: %+v", cfg)
	}
}

func TestUpdateInspectionScheduleSyncPersistsSmallFile(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(filepath.Join(dir, "results.json"))
	resetStoreIOForTest()
	engine.mu.Lock()
	old := engine.schedule
	engine.schedule = defaultInspectionSchedule()
	// Put a huge results list so a wrong implementation would rewrite it.
	engine.results = make([]accountResult, 200)
	for i := range engine.results {
		engine.results[i] = accountResult{AuthIndex: fmt.Sprintf("a%d", i), Classification: "healthy"}
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.waitAsyncPersist()
		engine.mu.Lock()
		engine.schedule = old
		engine.results = nil
		engine.mu.Unlock()
		setStoreFilePathForTest("")
		resetStoreIOForTest()
	})

	enabled := true
	interval := 3
	cfg, err := updateInspectionSchedule(inspectionScheduleUpdate{
		Enabled: &enabled, IntervalMinutes: &interval,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !cfg.Enabled || cfg.IntervalMinutes != 3 {
		t.Fatalf("cfg=%+v", cfg)
	}
	// Must exist on disk without waiting async results persist.
	raw, err := os.ReadFile(scheduleFilePath())
	if err != nil {
		t.Fatalf("schedule file missing: %v", err)
	}
	var disk persistedInspectionSchedule
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if !disk.Enabled || disk.IntervalMinutes != 3 {
		t.Fatalf("disk=%+v", disk)
	}
	// results.json may not yet contain schedule; small file is source of truth.
}

func TestLoadScheduleMigratesFromResultsJSON(t *testing.T) {
	dir := t.TempDir()
	resultsPath := filepath.Join(dir, "results.json")
	setStoreFilePathForTest(resultsPath)
	resetStoreIOForTest()
	t.Cleanup(func() {
		setStoreFilePathForTest("")
		resetStoreIOForTest()
	})
	snap := persistedSnapshot{
		Version: storeVersion,
		Schedule: persistedInspectionSchedule{
			Enabled: true, IntervalMinutes: 7, Workers: 2,
			PermissionDeniedAction: "disable", SpendingLimitAction: "disable",
		},
		Results: []accountResult{},
		SavedAt: time.Now().Format(time.RFC3339),
	}
	raw, _ := json.Marshal(snap)
	if err := os.WriteFile(resultsPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadInspectionScheduleFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || cfg.IntervalMinutes != 7 {
		t.Fatalf("migrated schedule = %+v", cfg)
	}
}

// ---------- 9) reset timestamp ms/s ----------

func TestAbsoluteResetTimeAcceptsSecondsAndMillis(t *testing.T) {
	now := time.Now()
	sec := now.Add(2 * time.Hour).Unix()
	ms := now.Add(3 * time.Hour).UnixMilli()
	hSec := http.Header{}
	hSec.Set("X-RateLimit-Reset-At", fmt.Sprintf("%d", sec))
	got, ok := absoluteResetTime(hSec, now)
	if !ok || got.Unix() != sec {
		t.Fatalf("seconds: ok=%v got=%v want=%d", ok, got, sec)
	}
	hMs := http.Header{}
	hMs.Set("X-RateLimit-Reset-At", fmt.Sprintf("%d", ms))
	got, ok = absoluteResetTime(hMs, now)
	if !ok || got.UnixMilli() != ms {
		t.Fatalf("millis: ok=%v got=%v want_ms=%d", ok, got, ms)
	}
}

func TestAbsoluteResetTimeRejectsAbsurdValues(t *testing.T) {
	// Far future years out / already expired unix values should be rejected by resolveResetAt path.
	now := time.Now()
	// 10-digit seconds far past
	hPast := http.Header{}
	hPast.Set("X-RateLimit-Reset-At", "100")
	if _, ok := absoluteResetTime(hPast, now); ok {
		t.Fatal("past unix reset must be rejected")
	}
	resetAt, src, ok2 := resolveBanWindow(429, exhaustedErrorCode, hPast, now, defaultPluginConfig())
	if !ok2 || !resetAt.After(now) {
		t.Fatalf("past absolute must fall back to future window: %v ok=%v src=%s", resetAt, ok2, src)
	}
	// Absurd far-future ms (year ~5000+)
	hFar := http.Header{}
	hFar.Set("X-RateLimit-Reset-At", "99999999999999")
	if got, ok := absoluteResetTime(hFar, now); ok {
		t.Fatalf("absurd far future accepted: %v", got)
	}
}

// ---------- 10) 401 category consistency ----------

func TestBanCategoryMaps401BodyCodesToUnauthorized(t *testing.T) {
	for _, code := range []string{
		"authentication_error", "invalid_token", "UNAUTHORIZED", "unauthorized", "token_expired",
	} {
		if cat := banCategoryOf(code); cat != "unauthorized" {
			t.Fatalf("code %q category=%q want unauthorized", code, cat)
		}
	}
}

func TestDetect401KeepsDiagnosticCodeButUnauthorizedCategory(t *testing.T) {
	cfg := defaultPluginConfig()
	record := pluginapi.UsageRecord{
		Provider: "xai", AuthID: "xai-401", Failed: true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 401,
			Body:       `{"code":"authentication_error","error":"token expired"}`,
		},
	}
	entry, ok := detectBan(record, cfg, time.Now())
	if !ok {
		t.Fatal("401 must ban")
	}
	if entry.ErrorCode != unauthorizedErrorCode {
		t.Fatalf("visible error_code must be unauthorized, got %q", entry.ErrorCode)
	}
	if entry.ErrorCodeDiag != "authentication_error" {
		t.Fatalf("diagnostic code = %q", entry.ErrorCodeDiag)
	}
	if banCategoryOf(entry.ErrorCode) != "unauthorized" {
		t.Fatalf("category = %s", banCategoryOf(entry.ErrorCode))
	}
}

// ---------- Non-regression anchors ----------

func TestClassifyBare429AndExact402Unchanged(t *testing.T) {
	got429 := classifyProbe(classifyInput{Lang: LangZH, ChatStatus: 429, ChatError: "rate limit"})
	if got429.Classification != "probe_error" || got429.Action == "disable" {
		t.Fatalf("bare 429 must not disable: %+v", got429)
	}
	if got := classifyProbe(classifyInput{
		Lang: LangZH, ChatStatus: 402,
		ChatCode: spendingLimitErrorCode, ChatError: "out of credits",
	}); got.Classification != "spending_limit" {
		t.Fatalf("exact 402: %+v", got)
	}
	if got := classifyProbe(classifyInput{
		Lang: LangZH, ChatStatus: 402, ChatCode: "other-402", ChatError: "nope",
	}); got.Classification == "spending_limit" || got.Classification == "permission_denied" {
		t.Fatalf("unknown 402 must not be actionable: %+v", got)
	}
}

func TestEngineShutdownSourceStopsBanDisposeWorkers(t *testing.T) {
	raw, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	idx := strings.Index(src, "func (e *inspectionEngine) shutdown()")
	if idx < 0 {
		t.Fatal("shutdown missing")
	}
	end := idx + 1200
	if end > len(src) {
		end = len(src)
	}
	chunk := src[idx:end]
	if !strings.Contains(chunk, "stopBanDisposeWorkers()") {
		t.Fatalf("engine.shutdown must stop ban dispose workers: %q", chunk)
	}
	raw2, err := os.ReadFile("cgo_bridge.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw2), "stopBanDisposeWorkers()") {
		t.Fatal("cliproxyPluginShutdown must stop ban dispose workers")
	}
}
