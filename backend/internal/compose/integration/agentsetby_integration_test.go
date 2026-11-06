// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A rule says which passport it stops with, while one stands behind it. A user
// editing the rule makes it theirs, and the field goes.
func TestARuleNamesThePassportThatTurnedItOn(t *testing.T) {
	d := newTwoDoors(t, "agent-rule-setter", "read", "write")
	rule := answerID(t, settingsAct(t, d, "create_automation", map[string]any{"request_body": AnyMap{
		"key": "assign_lead_owner", "name": "Assign leads", "params": AnyMap{},
	}}))
	var read struct {
		Setter *string `json:"setter_passport_id"`
	}
	if status := d.e.Call(t, "PATCH", "/v1/automations/"+rule, AnyMap{"status": "enabled"}, d.bearer, &read); status != http.StatusOK || read.Setter == nil {
		t.Fatalf("the agent's enable → %d, setter %v", status, read.Setter)
	}
	var stored string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT setter_passport_id::text FROM automation WHERE id = $1`, rule).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if *read.Setter != stored {
		t.Errorf("the wire names %s, the rule stores %s", *read.Setter, stored)
	}
	read.Setter = nil
	if status := d.e.Call(t, "PATCH", "/v1/automations/"+rule, AnyMap{"name": "Mine now"}, nil, &read); status != http.StatusOK {
		t.Fatalf("the user's edit → %d", status)
	}
	if read.Setter != nil {
		t.Errorf("after the user's edit the rule still names %s", *read.Setter)
	}
}

// The user's own seat says an agent set the zone until the user chooses. The roster
// never carries it, so a colleague does not learn who works through an agent.
func TestATimeZoneSaysItsAgentOnTheUsersOwnSeatOnly(t *testing.T) {
	d := newTwoDoors(t, "agent-zone-wire", "read", "write")
	ownAct(t, d, "save_timezone", map[string]any{"request_body": AnyMap{"timezone": "Europe/Berlin", "source": "agent"}})
	setBy := func() *string {
		t.Helper()
		var me struct {
			User struct {
				SetBy *string `json:"timezone_set_by"`
			} `json:"user"`
		}
		if status := d.e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
			t.Fatalf("GET /v1/me → %d", status)
		}
		return me.User.SetBy
	}
	if got := setBy(); got == nil || !isAgentStamp(*got) {
		t.Errorf("the seat reads timezone_set_by %v, want the agent's stamp", got)
	}
	var roster json.RawMessage
	if status := d.e.Call(t, "GET", "/v1/users", nil, nil, &roster); status != http.StatusOK {
		t.Fatalf("GET /v1/users → %d", status)
	}
	if strings.Contains(string(roster), "timezone_set_by") {
		t.Errorf("the roster carries who set a colleague's zone: %s", roster)
	}
	if status := d.e.Call(t, "PUT", "/v1/me/timezone", AnyMap{"timezone": "Europe/Berlin", "source": "chosen"}, nil, nil); status != http.StatusOK {
		t.Fatalf("the user choosing → %d", status)
	}
	if got := setBy(); got != nil {
		t.Errorf("after the user chose, the seat reads timezone_set_by %s", *got)
	}
}

// A passport's body over the limit is told the limit on every route the gate fronts.
func TestAnAgentsBodyOverTheLimitNamesIt(t *testing.T) {
	d := newTwoDoors(t, "agent-body-limit", "read", "write")
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	status := d.e.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": strings.Repeat("x", 2<<20), "source": "manual"}, d.bearer, &problem)
	if status != http.StatusRequestEntityTooLarge || problem.Code != "body_too_large" || !strings.Contains(problem.Detail, "MB") {
		t.Errorf("a body over the limit answered %d %+v", status, problem)
	}
}
