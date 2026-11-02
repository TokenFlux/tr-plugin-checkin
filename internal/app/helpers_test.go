package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/plugin-sdk/go/plugintest"
	"github.com/TokenFlux/TokenRouter/plugin-sdk/go/sdk"
)

// helpers_test.go creates isolated memory-host fixtures shared by claim and
// history handler tests. Records pass through the same SDK KV API as production,
// while a fixed host day makes month boundaries and stored monetary intent
// independent of the machine clock running the tests.

// newTestPlugin initializes an isolated SDK host with a fixed November calendar day.
// Each test owns its harness and configuration. Callers can replace the clock
// or location before requests to reproduce calendar boundaries deterministically.
func newTestPlugin(t *testing.T) (*Plugin, *plugintest.Harness) {
	t.Helper()
	h := plugintest.New(t)
	config, err := sdk.NewConfig([]byte(`{"reward":0.1}`))
	if err != nil {
		t.Fatal(err)
	}
	h.Env.Config = config
	p := New()
	p.now = func() time.Time { return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) }
	if err := p.Init(context.Background(), h.Env); err != nil {
		t.Fatal(err)
	}
	return p, h
}

// seedRecord writes one daily fixture under its production recipient-and-date identity.
func seedRecord(t *testing.T, h *plugintest.Harness, record checkin) {
	t.Helper()
	key := fmt.Sprintf("%d:%s", record.UserID, record.Day)
	if _, err := h.Env.Host.KV.Put(context.Background(), "checkins", key, record); err != nil {
		t.Fatal(err)
	}
}
