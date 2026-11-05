// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
)

// A run whose seed search fails still completes, with only a log line to say so.
// So each shipped agent must reach the model carrying a record its seed query names.
func TestEachShippedAgentReachesTheModelSeeded(t *testing.T) {
	for _, spec := range runner.Catalog() {
		t.Run(spec.Name, func(t *testing.T) {
			alternatives := strings.Split(spec.SeedQuery, " or ")
			name := alternatives[len(alternatives)-1] + " Harbor Logistics"
			re := setupRunner(t)
			var company struct {
				ID string `json:"id"`
			}
			if status := re.Call(t, "POST", "/v1/companies", integration.AnyMap{
				"source": "manual", "display_name": name,
			}, nil, &company); status != http.StatusCreated {
				t.Fatalf("create company → %d", status)
			}

			re.brain.Script(`{"final":{"summary":"nothing to do"}}`)
			trigger := spec.Name + ":e2e-seeded"
			re.enqueue(t, spec.Name, trigger, &re.passportID)
			re.tick(t)

			calls := re.brain.Calls()
			if len(calls) == 0 {
				t.Fatal("the run never reached the model")
			}
			prompt := string(calls[0].Payload)
			if !strings.Contains(prompt, "Seed context") {
				t.Fatalf("the run reached the model with no seed context:\n%s", prompt)
			}
			if !strings.Contains(prompt, "company:"+company.ID) {
				t.Errorf("the seed context does not carry %s (company:%s):\n%s", name, company.ID, prompt)
			}
		})
	}
}
