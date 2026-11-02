package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/plugin-sdk/go/sdk"
)

// history_test.go exercises authenticated history handlers against the SDK memory
// host. Mixed recipients, month boundaries, pending intent, decimal precision,
// and multiple storage pages verify observable API behavior rather than duplicating
// the implementation's filtering or summary calculations inside test helpers.

// TestPersonalHistoryScopesIdentityAndPreservesCapturedRewards supplies another
// user's identifier and mixes pending and confirmed months. The response must
// retain only the authenticated user's records and sum actual completed amounts.
func TestPersonalHistoryScopesIdentityAndPreservesCapturedRewards(t *testing.T) {
	p, h := newTestPlugin(t)
	for _, record := range []checkin{
		{UserID: 7, Day: "2026-10-31", Amount: "0.1", Completed: true},
		{UserID: 7, Day: "2026-11-01", Amount: "0.2", Completed: true},
		{UserID: 7, Day: "2026-11-02", Amount: "0.3"},
		{UserID: 8, Day: "2026-11-01", Amount: "9", Completed: true},
	} {
		seedRecord(t, h, record)
	}
	response := h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, "/history?month=2026-11&scope=all&user_id=8", nil), sdk.Caller{UserID: 7, Role: "user"})
	if response.Code != http.StatusOK {
		t.Fatalf("response: %d %s", response.Code, response.Body.String())
	}
	var result historyResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || result.Summary.Count != 2 || result.Summary.Amount != "0.3" ||
		result.MonthSummary.Count != 1 || result.MonthSummary.Amount != "0.2" || len(result.Calendar) != 2 {
		t.Fatalf("unexpected personal history: %+v", result)
	}
	if result.Rows[0].Day != "2026-11-02" || result.Rows[0].Completed || result.Rows[2].Day != "2026-10-31" {
		t.Fatalf("incorrect ordering or pending state: %+v", result.Rows)
	}
	for _, row := range result.Rows {
		if row.UserID != 7 {
			t.Fatalf("cross-user record: %+v", row)
		}
	}
}

// TestPersonalHistoryTraversesStoragePagesWithoutRounding stores more than one
// host page of minimal monetary units. Table pagination must preserve newest-first
// ordering while the complete lifetime total retains all eight decimal places.
func TestPersonalHistoryTraversesStoragePagesWithoutRounding(t *testing.T) {
	p, h := newTestPlugin(t)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := range 505 {
		seedRecord(t, h, checkin{UserID: 7, Day: start.AddDate(0, 0, index).Format(time.DateOnly), Amount: "0.00000001", Completed: true})
	}
	response := h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, "/history?scope=all&page=2", nil), sdk.Caller{UserID: 7, Role: "user"})
	var result historyResponse
	if response.Code != http.StatusOK {
		t.Fatalf("response: %d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 505 || result.Summary.Amount != "0.00000505" || result.Page != 2 || len(result.Rows) != 10 ||
		result.Rows[0].Day != start.AddDate(0, 0, 494).Format(time.DateOnly) {
		t.Fatalf("incorrect paginated summary: %+v", result)
	}
}

// TestAdminHistoryRequiresRoleAndAppliesFilters attempts both direct unauthorized
// access and a legitimate filtered query. Only the administrator can read another
// recipient, and the response does not expose a redundant all-user calendar.
func TestAdminHistoryRequiresRoleAndAppliesFilters(t *testing.T) {
	p, h := newTestPlugin(t)
	seedRecord(t, h, checkin{UserID: 9, Day: "2026-10-31", Amount: "1", Completed: true})
	seedRecord(t, h, checkin{UserID: 9, Day: "2026-11-01", Amount: "0.2"})
	seedRecord(t, h, checkin{UserID: 8, Day: "2026-11-01", Amount: "0.3"})
	url := "/admin/history?user_id=9&month=2026-11&state=pending&page=999"
	response := h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, url, nil), sdk.Caller{UserID: 7, Role: "user"})
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-admin response: %d", response.Code)
	}
	response = h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, url, nil), sdk.Caller{UserID: 7, Role: "admin"})
	var result historyResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result.Total != 1 || len(result.Rows) != 1 || result.Rows[0].UserID != 9 ||
		result.Rows[0].Completed || result.Page != 1 || len(result.Calendar) != 0 {
		t.Fatalf("filtered response: %d %+v", response.Code, result)
	}
}

// TestHistoryRejectsInvalidFiltersAndUnauthenticatedReads sends malformed months,
// identifiers, states, and page values. Every invalid query must fail explicitly,
// while an absent caller must never fall back to an all-recipient history scan.
func TestHistoryRejectsInvalidFiltersAndUnauthenticatedReads(t *testing.T) {
	p, h := newTestPlugin(t)
	for _, query := range []string{"month=2026-13", "month=2026-1", "page=0", "page=abc", "user_id=-1", "state=other"} {
		response := h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, "/history?"+query, nil), sdk.Caller{UserID: 7})
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s returned %d", query, response.Code)
		}
	}
	response := h.Request(p.Handler(), httptest.NewRequest(http.MethodGet, "/history", nil), sdk.Caller{})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous response: %d", response.Code)
	}
}
