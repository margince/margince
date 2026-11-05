// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A record with nothing in it still answers the shapes the contract declares.
// An array is declared where it is read, and it is never null.
func TestAnEmptyRecordReadsAsDeclaredArraysNotNull(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Read Shapes", "admin@readshapes.test", "Admin")

	t.Run("a company with no relationship type", func(t *testing.T) {
		id := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Bare Works", "source": "manual"})
		var page struct {
			StateStrip struct {
				Account struct {
					RelationshipTypes json.RawMessage `json:"relationship_types"`
				} `json:"account"`
			} `json:"state_strip"`
		}
		if status := e.Call(t, http.MethodGet, "/v1/companies/"+id+"/360", nil, nil, &page); status != http.StatusOK {
			t.Fatalf("GET company 360 → %d", status)
		}
		if got := string(page.StateStrip.Account.RelationshipTypes); got != "[]" {
			t.Errorf("state_strip.account.relationship_types = %s, want []", got)
		}
	})

	t.Run("a text question in a review template", func(t *testing.T) {
		var templates struct {
			Data []struct {
				Key       string `json:"key"`
				Questions []map[string]json.RawMessage
			} `json:"data"`
		}
		if status := e.Call(t, http.MethodGet, "/v1/activity-review-templates", nil, nil, &templates); status != http.StatusOK {
			t.Fatalf("GET activity-review-templates → %d", status)
		}
		textQuestions := 0
		for _, template := range templates.Data {
			for _, question := range template.Questions {
				if string(question["type"]) != `"text"` {
					continue
				}
				textQuestions++
				if options, present := question["options"]; present {
					t.Errorf("template %s: text question %s carries options %s, want the key left off", template.Key, question["key"], options)
				}
			}
		}
		if textQuestions == 0 {
			t.Fatal("no seeded template has a text question, so this proves nothing")
		}
	})
}
