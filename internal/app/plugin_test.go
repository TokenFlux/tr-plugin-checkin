package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/TokenFlux/TokenRouter/plugin-sdk/go/plugintest"
	"github.com/TokenFlux/TokenRouter/plugin-sdk/go/sdk"
)

// plugin_test.go verifies daily reward retry behavior against the SDK memory host.
// A failed credit preserves the claimed reward, and retrying after configuration
// changes must issue that original amount exactly once before rejecting later
// completed claims for the same user and day.

// TestCheckinRetriesPendingRewardAfterHostFailure reproduces a credit failure
// after the daily KV claim succeeds. Retrying must use the captured reward,
// grant it once, and reject subsequent completed check-ins.
func TestCheckinRetriesPendingRewardAfterHostFailure(t *testing.T) {
	h := plugintest.New(t)
	if err := h.Server.SetBalance(7, "1"); err != nil {
		t.Fatal(err)
	}
	config, err := sdk.NewConfig([]byte(`{"reward":0.1}`))
	if err != nil {
		t.Fatal(err)
	}
	h.Env.Config = config
	p := New()
	if err := p.Init(context.Background(), h.Env); err != nil {
		t.Fatal(err)
	}
	caller := sdk.Caller{UserID: 7, Username: "tester", Role: "user"}
	h.Server.Fail("WalletCredit", plugintest.Error(codes.Unavailable, "HOST_UNAVAILABLE"))
	response := h.Request(p.Handler(), httptest.NewRequest(http.MethodPost, "/checkin", nil), caller)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("first response: %d", response.Code)
	}
	h.Server.Fail("WalletCredit", nil)
	changed, err := sdk.NewConfig([]byte(`{"reward":0.5}`))
	if err != nil {
		t.Fatal(err)
	}
	h.Env.Config = changed
	response = h.Request(p.Handler(), httptest.NewRequest(http.MethodPost, "/checkin", nil), caller)
	if response.Code != http.StatusOK {
		t.Fatalf("retry response: %d %s", response.Code, response.Body.String())
	}
	response = h.Request(p.Handler(), httptest.NewRequest(http.MethodPost, "/checkin", nil), caller)
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate response: %d", response.Code)
	}
	wallet, err := h.Env.Host.Wallet.Get(context.Background(), 7)
	if err != nil || wallet.Balance != "1.10000000" {
		t.Fatalf("balance: %v %v", wallet, err)
	}
}

// TestStatusKeepsCapturedAmountAndHostDSTBoundary changes the configured reward
// after a completed claim during a daylight-saving transition. Status must show
// the stored reward and the next host midnight, not a fixed twenty-four-hour day.
func TestStatusKeepsCapturedAmountAndHostDSTBoundary(t *testing.T) {
	p, h := newTestPlugin(t)
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	p.location = location
	p.now = func() time.Time { return time.Date(2026, 11, 1, 4, 30, 0, 0, time.UTC) }
	seedRecord(t, h, checkin{UserID: 7, Day: "2026-11-01", Amount: "1.25", Completed: true})
	response := h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, "/status", nil), sdk.Caller{UserID: 7})
	var result struct {
		Reward string `json:"reward"`        // Captured daily reward amount.
		Day    string `json:"day"`           // Current host calendar date.
		State  string `json:"state"`         // Current claim completion state.
		Reset  int64  `json:"next_reset_at"` // Next host midnight timestamp.
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result.Reward != "1.25" || result.Day != "2026-11-01" || result.State != "completed" ||
		result.Reset != time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC).Unix() {
		t.Fatalf("incorrect status: %d %+v", response.Code, result)
	}
}

// TestPausedConfigurationPreservesPendingIntentAndHistory pauses new claims with
// a subsequently invalid reward configuration. Previously accepted pending intent
// must still finish with its captured amount, and personal history remains readable.
func TestPausedConfigurationPreservesPendingIntentAndHistory(t *testing.T) {
	p, h := newTestPlugin(t)
	if err := h.Server.SetBalance(7, "1"); err != nil {
		t.Fatal(err)
	}
	seedRecord(t, h, checkin{UserID: 7, Day: "2026-11-02", Amount: "0.2"})
	config, err := sdk.NewConfig([]byte(`{"reward":0.000000001,"paused":true}`))
	if err != nil {
		t.Fatal(err)
	}
	h.Env.Config = config
	response := h.Request(p.Handler(), httptest.NewRequest(http.MethodPost, "/checkin", nil), sdk.Caller{UserID: 8})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("new paused claim: %d", response.Code)
	}
	missing, err := h.Env.Host.KV.Get(context.Background(), "checkins", "8:2026-11-02")
	if err != nil || missing.Found {
		t.Fatalf("paused claim created intent: %+v %v", missing, err)
	}
	response = h.Request(p.Handler(), httptest.NewRequest(http.MethodPost, "/checkin", nil), sdk.Caller{UserID: 7})
	if response.Code != http.StatusOK {
		t.Fatalf("pending retry: %d %s", response.Code, response.Body.String())
	}
	wallet, err := h.Env.Host.Wallet.Get(context.Background(), 7)
	if err != nil || wallet.Balance != "1.20000000" {
		t.Fatalf("captured reward: %+v %v", wallet, err)
	}
	response = h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, "/history", nil), sdk.Caller{UserID: 7})
	if response.Code != http.StatusOK {
		t.Fatalf("paused history: %d", response.Code)
	}
}

// TestZeroRewardRecordsCompletionWithoutWalletCredit signs in with no monetary
// reward and no initialized wallet. Completion and history must succeed without
// calling the unavailable wallet capability or reporting a positive reward.
func TestZeroRewardRecordsCompletionWithoutWalletCredit(t *testing.T) {
	p, h := newTestPlugin(t)
	config, err := sdk.NewConfig([]byte(`{"reward":0}`))
	if err != nil {
		t.Fatal(err)
	}
	h.Env.Config = config
	h.Server.Fail("WalletCredit", plugintest.Error(codes.Unavailable, "HOST_UNAVAILABLE"))
	response := h.Request(p.Handler(), httptest.NewRequest(http.MethodPost, "/checkin", nil), sdk.Caller{UserID: 7})
	if response.Code != http.StatusOK {
		t.Fatalf("zero reward response: %d %s", response.Code, response.Body.String())
	}
	response = h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, "/history", nil), sdk.Caller{UserID: 7})
	var result historyResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Count != 1 || result.Summary.Amount != "0" || len(result.Rows) != 1 || !result.Rows[0].Completed {
		t.Fatalf("zero reward history: %+v", result)
	}
}

// TestConcurrentCheckinsCreditOnce sends overlapping claims through one plugin
// instance. Every successful or duplicate response must refer to the same daily
// intent, and the final wallet balance must contain exactly one reward.
func TestConcurrentCheckinsCreditOnce(t *testing.T) {
	p, h := newTestPlugin(t)
	if err := h.Server.SetBalance(7, "1"); err != nil {
		t.Fatal(err)
	}
	results := make(chan int, 12)
	var group sync.WaitGroup
	// Collect every response before checking the wallet so the assertion observes
	// all competing finalizations, including requests replaying the winning credit.
	for range 12 {
		group.Go(func() {
			response := h.Request(p.Handler(), httptest.NewRequest(http.MethodPost, "/checkin", nil), sdk.Caller{UserID: 7})
			results <- response.Code
		})
	}
	group.Wait()
	close(results)
	for code := range results {
		if code != http.StatusOK && code != http.StatusConflict {
			t.Fatalf("concurrent response: %d", code)
		}
	}
	wallet, err := h.Env.Host.Wallet.Get(context.Background(), 7)
	if err != nil || wallet.Balance != "1.10000000" {
		t.Fatalf("concurrent balance: %+v %v", wallet, err)
	}
}

// TestFinalizationFailureReplaysOriginalCredit interrupts the KV completion write
// after a wallet credit succeeds. Retrying must reconcile the original intent,
// preserve its amount after configuration changes, and never pay it a second time.
func TestFinalizationFailureReplaysOriginalCredit(t *testing.T) {
	p, h := newTestPlugin(t)
	if err := h.Server.SetBalance(7, "1"); err != nil {
		t.Fatal(err)
	}
	seedRecord(t, h, checkin{UserID: 7, Day: "2026-11-02", Amount: "0.2"})
	h.Server.Fail("KVPut", plugintest.Error(codes.Unavailable, "HOST_UNAVAILABLE"))
	response := h.Request(p.Handler(), httptest.NewRequest(http.MethodPost, "/checkin", nil), sdk.Caller{UserID: 7})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("failed finalization: %d", response.Code)
	}
	h.Server.Fail("KVPut", nil)
	response = h.Request(p.Handler(), httptest.NewRequest(http.MethodPost, "/checkin", nil), sdk.Caller{UserID: 7})
	if response.Code != http.StatusConflict {
		t.Fatalf("replayed finalization: %d %s", response.Code, response.Body.String())
	}
	wallet, err := h.Env.Host.Wallet.Get(context.Background(), 7)
	if err != nil || wallet.Balance != "1.20000000" {
		t.Fatalf("replayed balance: %+v %v", wallet, err)
	}
	stored, err := h.Env.Host.KV.Get(context.Background(), "checkins", "7:2026-11-02")
	if err != nil {
		t.Fatal(err)
	}
	var record checkin
	if err := json.Unmarshal(stored.ValueJson, &record); err != nil || !record.Completed || record.Amount != "0.2" {
		t.Fatalf("recovered intent: %+v %v", record, err)
	}
}

// TestSettingsOverviewRequiresAdministrator checks the plugin's own authorization
// boundary independently of manifest routing. Only an administrator receives the
// current nonsecret reward rules, and the overview declares no mutation endpoint.
func TestSettingsOverviewRequiresAdministrator(t *testing.T) {
	p, h := newTestPlugin(t)
	response := h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, "/admin/settings", nil), sdk.Caller{UserID: 7, Role: "user"})
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-admin settings response: %d", response.Code)
	}
	response = h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, "/admin/settings", nil), sdk.Caller{UserID: 7, Role: "admin"})
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result["reward"] != "0.1" || result["paused"] != false || len(result) != 4 {
		t.Fatalf("settings overview: %d %+v", response.Code, result)
	}
	response = h.Request(p.Handler(), httptest.NewRequest(http.MethodPut, "/admin/settings", nil), sdk.Caller{UserID: 7, Role: "admin"})
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected settings mutation route: %d", response.Code)
	}
}
