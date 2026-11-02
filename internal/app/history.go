package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/plugin-sdk/go/pluginpb/v1"
	"github.com/TokenFlux/TokenRouter/plugin-sdk/go/sdk"
)

// history.go serves personal calendars, exact reward totals, and filtered
// administrator records from the existing daily KV collection. Personal reads
// select the verified caller's key prefix before accessing storage. Sorting and
// filtering happen on the server, and only the requested table page and selected
// calendar month leave the process. Completed zero rewards count as recorded
// days but do not claim a wallet credit occurred.
//
// The host exposes ascending key pagination without date indexes or aggregation.
// Reads therefore traverse bounded pages, reject oversized or incomplete results,
// and accumulate decimal amounts exactly. No secondary persistent keys are added,
// so historical rows remain compatible and indexing does not consume the plugin's
// existing key quota. Concurrent claims may appear on the following refresh;
// this read path does not promise a multi-page database transaction snapshot.

const (
	// History scans follow the host's page maximum and bound memory at its nominal
	// namespace key quota; a soft-quota overshoot fails instead of truncating totals.
	historyBatchSize   = 500
	historyRecordLimit = 50000

	// historyPageSize bounds table payloads while leaving the personal calendar and
	// both reward summaries complete for the authorized record scope.
	historyPageSize = 10
)

var (
	// errHistoryTooLarge prevents partial scans from masquerading as complete account
	// totals when a namespace exceeds the amount this reader can safely retain.
	errHistoryTooLarge = errors.New("check-in history exceeds the supported record limit")
)

// historyQuery contains validated calendar, recipient, completion, and pagination filters.
type historyQuery struct {
	Month         string // Empty selects every month.
	CalendarMonth string // Personal calendar month; empty omits it.
	UserID        int64  // Zero selects all authorized recipients.
	State         string // Empty, completed, or pending.
	Page          int    // One-based requested table page.
}

// historySummary counts completed days and sums their captured decimal rewards.
type historySummary struct {
	Count  int    `json:"count"`  // Confirmed daily record count.
	Amount string `json:"amount"` // Exact internal balance units.
}

// historyResponse separates paginated rows from a complete selected-month calendar.
type historyResponse struct {
	Rows         []checkin      `json:"rows"`          // Selected table page records.
	Calendar     []checkin      `json:"calendar"`      // All records in selected month.
	Months       []string       `json:"months"`        // Recorded months, newest first.
	Month        string         `json:"month"`         // Selected calendar month identifier.
	Total        int            `json:"total"`         // Matching records before pagination.
	Page         int            `json:"page"`          // Resolved one-based table page.
	PageSize     int            `json:"page_size"`     // Maximum returned table rows.
	Summary      historySummary `json:"summary"`       // All completed authorized records.
	MonthSummary historySummary `json:"month_summary"` // Completed selected-month records.
}

// personalHistory reads only the verified caller's records, regardless of query identity.
// Month omission selects the current host month. The scope filter can request
// all personal table rows while leaving the calendar on its selected month.
func (p *Plugin) personalHistory(w http.ResponseWriter, r *http.Request) {
	caller, ok := sdk.CallerFrom(r.Context())
	if !ok || caller.UserID == 0 {
		sdk.WriteJSON(w, http.StatusUnauthorized, map[string]string{"code": "LOGIN_REQUIRED"})
		return
	}
	query, err := parseHistoryQuery(r)
	if err != nil {
		sdk.WriteError(w, err)
		return
	}
	query.UserID = caller.UserID
	if query.Month == "" {
		query.Month = p.now().In(p.location).Format("2006-01")
	}
	query.CalendarMonth = query.Month
	if r.URL.Query().Get("scope") == "all" {
		query.Month = ""
	}
	p.writeHistory(w, r, query)
}

// adminHistory checks administrator identity before exposing cross-user reward records.
// Missing month selects all dates. User, month, and state filters remain explicit
// so pagination counts describe the same set as the returned rows.
func (p *Plugin) adminHistory(w http.ResponseWriter, r *http.Request) {
	caller, ok := sdk.CallerFrom(r.Context())
	if !ok || caller.UserID == 0 || caller.Role != "admin" {
		sdk.WriteJSON(w, http.StatusForbidden, map[string]string{"code": "ADMIN_REQUIRED"})
		return
	}
	query, err := parseHistoryQuery(r)
	if err != nil {
		sdk.WriteError(w, err)
		return
	}
	p.writeHistory(w, r, query)
}

// parseHistoryQuery rejects malformed filters before any host storage reads occur.
// Empty filters preserve their documented unfiltered meaning. Page values are
// bounded to keep arithmetic and user-visible pagination within supported history.
func parseHistoryQuery(r *http.Request) (historyQuery, error) {
	values := r.URL.Query()
	query := historyQuery{Month: values.Get("month"), State: values.Get("state"), Page: 1}
	if query.Month != "" {
		month, err := time.Parse("2006-01", query.Month)
		if err != nil || month.Format("2006-01") != query.Month {
			return query, sdk.ErrInvalidArgument
		}
	}
	if query.State != "" && query.State != "completed" && query.State != "pending" {
		return query, sdk.ErrInvalidArgument
	}
	if value := values.Get("user_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return query, sdk.ErrInvalidArgument
		}
		query.UserID = id
	}
	if value := values.Get("page"); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil || page < 1 || page > historyRecordLimit {
			return query, sdk.ErrInvalidArgument
		}
		query.Page = page
	}
	return query, nil
}

// writeHistory combines a bounded record scan with exact summaries and page selection.
// Errors return no partial calendar or totals. Personal all-month table mode only
// widens the date filter; the already validated recipient restriction stays intact.
func (p *Plugin) writeHistory(w http.ResponseWriter, r *http.Request, query historyQuery) {
	prefix := ""
	if query.UserID != 0 {
		prefix = strconv.FormatInt(query.UserID, 10) + ":"
	}
	records, err := p.readRecords(r.Context(), prefix)
	if errors.Is(err, errHistoryTooLarge) {
		sdk.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "HISTORY_TOO_LARGE"})
		return
	}
	if err != nil {
		sdk.WriteError(w, err)
		return
	}
	sdk.WriteJSON(w, http.StatusOK, selectHistory(records, query))
}

// readRecords traverses host pages and validates record identity before publication.
// A personal prefix bounds storage access, not just output filtering. Any invalid
// record or nonadvancing cursor fails the complete read instead of hiding records.
//
// Storage keys and payload identities are checked together before rows enter the
// response. This prevents malformed persisted data from attaching another user's
// amount to a caller's date. The caller owns the returned slice, and subsequent
// sorting does not modify host KV or any independently progressing request.
func (p *Plugin) readRecords(ctx context.Context, prefix string) ([]checkin, error) {
	records := make([]checkin, 0)
	cursor := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := p.env.Host.KV.List(ctx, &pluginpb.KVListRequest{
			Collection: "checkins",
			Prefix:     prefix,
			Cursor:     cursor,
			Limit:      historyBatchSize,
		})
		if err != nil {
			return nil, err
		}
		if len(records)+len(page.Items) > historyRecordLimit {
			return nil, errHistoryTooLarge
		}
		for _, item := range page.Items {
			var record checkin
			if err := json.Unmarshal(item.ValueJson, &record); err != nil {
				return nil, err
			}
			day, err := time.Parse(time.DateOnly, record.Day)
			if err != nil || day.Format(time.DateOnly) != record.Day || record.UserID <= 0 ||
				item.Key != fmt.Sprintf("%d:%s", record.UserID, record.Day) || !strings.HasPrefix(item.Key, prefix) {
				return nil, fmt.Errorf("invalid stored check-in identity")
			}
			if record.Amount != "0" {
				if _, err := sdk.ParseAmount(record.Amount); err != nil {
					return nil, err
				}
			}
			records = append(records, record)
		}
		if page.NextCursor == "" {
			return records, nil
		}
		if page.NextCursor <= cursor {
			return nil, fmt.Errorf("history cursor did not advance")
		}
		cursor = page.NextCursor
	}
}

// selectHistory sorts records and computes totals from confirmed financial intent.
// Monthly and lifetime totals deliberately ignore table completion filters, while
// the matching count includes pending rows when requested. Callers provide valid
// decimal amounts and dates through readRecords before using this calculation.
// The supplied slice belongs to the caller and is reordered in place.
//
// Amounts are summed as exact rationals, preserving each claim's original value
// across configuration changes. Table pagination never truncates the calendar or
// the summaries. Out-of-range pages resolve to the last existing page so deleting
// records or narrowing a filter cannot strand a browser on an unreachable page.
func selectHistory(records []checkin, query historyQuery) historyResponse {
	slices.SortFunc(records, func(a, b checkin) int {
		if a.Day != b.Day {
			return strings.Compare(b.Day, a.Day)
		}
		if a.At != b.At {
			if a.At > b.At {
				return -1
			}
			return 1
		}
		if a.UserID < b.UserID {
			return -1
		}
		if a.UserID > b.UserID {
			return 1
		}
		return 0
	})
	result := historyResponse{
		Rows:     []checkin{},
		Calendar: []checkin{},
		Months:   []string{},
		Month:    query.CalendarMonth,
		PageSize: historyPageSize,
	}
	matching := make([]checkin, 0, len(records))
	months := make(map[string]bool)
	totalAmount, monthAmount := new(big.Rat), new(big.Rat)
	for _, record := range records {
		month := record.Day[:7]
		if !months[month] {
			result.Months = append(result.Months, month)
			months[month] = true
		}
		inMonth := month == query.CalendarMonth
		if inMonth {
			result.Calendar = append(result.Calendar, record)
		}
		if record.Completed {
			amount, _ := new(big.Rat).SetString(record.Amount)
			totalAmount.Add(totalAmount, amount)
			result.Summary.Count++
			if inMonth {
				monthAmount.Add(monthAmount, amount)
				result.MonthSummary.Count++
			}
		}
		if query.Month != "" && month != query.Month {
			continue
		}
		if (query.State == "completed" && !record.Completed) || (query.State == "pending" && record.Completed) {
			continue
		}
		matching = append(matching, record)
	}
	// Eight decimal places retain the wallet protocol's complete precision; trimming
	// insignificant zeros changes presentation without rounding stored reward amounts.
	result.Summary.Amount = strings.TrimRight(strings.TrimRight(totalAmount.FloatString(8), "0"), ".")
	result.MonthSummary.Amount = strings.TrimRight(strings.TrimRight(monthAmount.FloatString(8), "0"), ".")
	result.Total = len(matching)
	result.Page = min(query.Page, max(1, (len(matching)+historyPageSize-1)/historyPageSize))
	start := (result.Page - 1) * historyPageSize
	result.Rows = matching[start:min(start+historyPageSize, len(matching))]
	return result
}
