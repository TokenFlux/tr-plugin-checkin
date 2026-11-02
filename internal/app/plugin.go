package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/TokenFlux/TokenRouter/plugin-sdk/go/sdk"
)

// plugin.go implements the plugin's daily balance reward using host storage.
// A pending KV record captures the day's reward before the idempotent wallet
// call, so a failed or uncertain credit can be retried with the original
// amount even if an administrator changes configuration. A completed record
// rejects later check-ins without issuing another wallet operation.
//
// KV and wallet writes are separate host calls and cannot share a transaction.
// The wallet idempotency key therefore carries the financial identity across
// retries, while compare-and-swap finalization records completion. A replayed
// wallet result may finish a previously interrupted pending record but never
// creates a second credit. Durable history lives in host KV, and the local
// state directory is not used for business data or backup requirements.

// Plugin implements authenticated daily rewards, personal history, and administrator reads.
type Plugin struct {
	env      *sdk.Env         // initialized host capability environment
	mux      *http.ServeMux   // declared plugin HTTP handlers
	location *time.Location   // host natural-day timezone
	now      func() time.Time // testable current time source
}

// checkin records one user's fixed reward and completion for a host-calendar day.
type checkin struct {
	UserID    int64  `json:"user_id"`   // recipient account identifier
	Day       string `json:"day"`       // host-calendar check-in date
	Amount    string `json:"amount"`    // fixed decimal reward amount
	Completed bool   `json:"completed"` // confirmed daily completion flag
	At        int64  `json:"at"`        // initial check-in Unix timestamp
}

// New constructs the check-in plugin without starting work or calling the host.
func New() *Plugin { return &Plugin{mux: http.NewServeMux(), now: time.Now} }

// Init resolves host timezone and installs the plugin's declared HTTP handlers.
// Failure to read site context prevents startup because daily boundaries must
// match the host's accounting timezone rather than the developer's local machine.
func (p *Plugin) Init(ctx context.Context, env *sdk.Env) error {
	site, err := env.Host.Site.Info(ctx)
	if err != nil {
		return err
	}
	location, err := time.LoadLocation(site.Timezone)
	if err != nil {
		return err
	}
	p.env = env
	p.location = location
	p.mux.HandleFunc("GET /status", p.status)
	p.mux.HandleFunc("POST /checkin", p.checkin)
	p.mux.HandleFunc("GET /history", p.personalHistory)
	p.mux.HandleFunc("GET /admin/history", p.adminHistory)
	p.mux.HandleFunc("GET /admin/settings", p.settings)
	return nil
}

// Handler returns the already initialized concurrent HTTP request multiplexer.
func (p *Plugin) Handler() http.Handler { return p.mux }

// ConfigChanged accepts validated settings already published through Env.Config.
// Pending rewards retain their captured amounts, while the next new daily
// record reads the latest configured value without requiring a process restart.
func (*Plugin) ConfigChanged(context.Context, sdk.Config) error { return nil }

// status returns today's recorded reward or the amount for a new claim.
// Authenticated reads never create records. Pending and completed records keep
// their captured amount after configuration changes; reset times use host midnight.
//
// Existing intent takes precedence over a later pause, allowing the browser to
// distinguish recoverable pending work from an unavailable new claim. The next
// midnight is constructed in the host location rather than adding twenty-four
// hours, because daylight-saving transitions can change a natural day's duration.
func (p *Plugin) status(w http.ResponseWriter, r *http.Request) {
	caller, ok := sdk.CallerFrom(r.Context())
	if !ok || caller.UserID == 0 {
		sdk.WriteJSON(w, http.StatusUnauthorized, map[string]string{"code": "LOGIN_REQUIRED"})
		return
	}
	now := p.now().In(p.location)
	day := now.Format(time.DateOnly)
	key := fmt.Sprintf("%d:%s", caller.UserID, day)
	value, err := p.env.Host.KV.Get(r.Context(), "checkins", key)
	if err != nil {
		sdk.WriteError(w, err)
		return
	}
	var record checkin
	if value.Found {
		if err := json.Unmarshal(value.ValueJson, &record); err != nil {
			sdk.WriteError(w, err)
			return
		}
	}
	reward := p.reward()
	state := "available"
	if p.env.Config.Bool("paused") {
		state = "paused"
	}
	if value.Found {
		reward = record.Amount
		state = "pending"
		if record.Completed {
			state = "completed"
		}
	}
	reset := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, p.location)
	sdk.WriteJSON(w, http.StatusOK, map[string]any{
		"checked_in":    record.Completed,
		"state":         state,
		"reward":        reward,
		"notice":        p.env.Config.String("notice"),
		"day":           day,
		"timezone":      p.location.String(),
		"server_time":   now.Unix(),
		"next_reset_at": reset.Unix(),
		"paused":        p.env.Config.Bool("paused"),
	})
}

// checkin captures a daily reward, credits it once, and records completion.
// Retrying a pending record reuses its stored amount and wallet key; completed
// records return a conflict without calling the wallet again.
// Pausing blocks new intent but permits previously accepted pending work to finish.
//
// A competing request may claim the KV key first, so the loser rereads that
// record before deciding whether work remains. Wallet replay distinguishes a
// recovered operation from a new credit. Finalization uses the claimed version
// and tolerates a competing completion, since the wallet ledger already proves
// the same financial intent was applied at most once.
func (p *Plugin) checkin(w http.ResponseWriter, r *http.Request) {
	caller, ok := sdk.CallerFrom(r.Context())
	if !ok || caller.UserID == 0 {
		sdk.WriteJSON(w, http.StatusUnauthorized, map[string]string{"code": "LOGIN_REQUIRED"})
		return
	}
	now := p.now().In(p.location)
	day := now.Format("2006-01-02")
	key := fmt.Sprintf("%d:%s", caller.UserID, day)
	record := checkin{
		UserID: caller.UserID,
		Day:    day,
		Amount: p.reward(),
		At:     now.Unix(),
	}
	value, err := p.env.Host.KV.Get(r.Context(), "checkins", key)
	if err != nil {
		sdk.WriteError(w, err)
		return
	}
	version := value.Version
	if !value.Found {
		if p.env.Config.Bool("paused") {
			sdk.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "CHECKIN_PAUSED"})
			return
		}
		if record.Amount != "0" {
			if _, err := sdk.ParseAmount(record.Amount); err != nil {
				sdk.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "INVALID_REWARD"})
				return
			}
		}
		// New intent is recorded only after current configuration passes monetary
		// validation; existing pending claims retain their previously accepted amount.
		version, err = p.env.Host.KV.Put(r.Context(), "checkins", key, record, sdk.IfAbsent())
	}
	if value.Found || errors.Is(err, sdk.ErrVersionConflict) {
		// Preserve the first claimant's amount when a retry or concurrent request
		// sees an existing record after configuration may have changed.
		value, readErr := p.env.Host.KV.Get(r.Context(), "checkins", key)
		if readErr != nil {
			sdk.WriteError(w, readErr)
			return
		}
		if !value.Found {
			sdk.WriteError(w, sdk.ErrVersionConflict)
			return
		}
		if err := json.Unmarshal(value.ValueJson, &record); err != nil {
			sdk.WriteError(w, err)
			return
		}
		version = value.Version
	} else if err != nil {
		sdk.WriteError(w, err)
		return
	}
	if record.Completed {
		sdk.WriteJSON(w, http.StatusConflict, map[string]string{"code": "ALREADY_CHECKED_IN"})
		return
	}
	replayed := false
	if record.Amount != "0" {
		credit, err := p.env.Host.Wallet.Credit(r.Context(), sdk.CreditRequest{
			UserID:         caller.UserID,
			Amount:         record.Amount,
			IdempotencyKey: "checkin:" + key,
			Reason:         "Daily check-in " + record.Day,
		})
		if err != nil {
			sdk.WriteError(w, err)
			return
		}
		replayed = credit.Replayed
	}
	record.Completed = true
	// Finalize only after a confirmed or replayed credit; a failed KV write
	// leaves the original amount available for safe wallet replay.
	_, err = p.env.Host.KV.Put(r.Context(), "checkins", key, record, sdk.IfVersion(version))
	if err != nil && !errors.Is(err, sdk.ErrVersionConflict) {
		sdk.WriteError(w, err)
		return
	}
	if errors.Is(err, sdk.ErrVersionConflict) && record.Amount == "0" {
		replayed = true
	}
	if replayed {
		sdk.WriteJSON(w, http.StatusConflict, map[string]string{"code": "ALREADY_CHECKED_IN"})
		return
	}
	sdk.WriteJSON(w, http.StatusOK, map[string]string{"reward": record.Amount, "day": record.Day})
}

// settings exposes only nonsecret reward rules to the authenticated administrator.
// Editing remains in the host configuration form so schema validation, step-up
// verification, audit, and release rollback keep a single authoritative owner.
func (p *Plugin) settings(w http.ResponseWriter, r *http.Request) {
	caller, ok := sdk.CallerFrom(r.Context())
	if !ok || caller.UserID == 0 || caller.Role != "admin" {
		sdk.WriteJSON(w, http.StatusForbidden, map[string]string{"code": "ADMIN_REQUIRED"})
		return
	}
	sdk.WriteJSON(w, http.StatusOK, map[string]any{
		"reward": p.reward(), "paused": p.env.Config.Bool("paused"),
		"notice": p.env.Config.String("notice"), "timezone": p.location.String(),
	})
}

// reward returns the host-configured amount without applying floating-point arithmetic.
func (p *Plugin) reward() string {
	return strconv.FormatFloat(p.env.Config.Float("reward"), 'f', -1, 64)
}
