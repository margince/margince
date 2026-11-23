// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentloope2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func plant(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const goodScenario = "name: annotates\nagent: morning_brief\nworklist: [deal_a]\ncriteria: [Names the evidence.]\n"

func TestScenariosLoadInNameOrder(t *testing.T) {
	dir := plant(t, map[string]string{
		"b.yaml": strings.Replace(goodScenario, "annotates", "zeta", 1),
		"a.yaml": strings.Replace(goodScenario, "annotates", "alpha", 1),
	})
	got, err := LoadScenarios(dir)
	if err != nil || len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Errorf("loaded %+v, %v", got, err)
	}
}

func TestAScenarioThatCannotBeJudgedIsRefused(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"no name":                   {strings.Replace(goodScenario, "name: annotates", "name: \"\"", 1), "no name"},
		"an agent the lane omits":   {strings.Replace(goodScenario, "morning_brief", "enrichment", 1), "not one the lane covers"},
		"writes expected, no list":  {strings.Replace(goodScenario, "worklist: [deal_a]", "worklist: []", 1), "names no worklist"},
		"no writes, but a list":     {goodScenario + "no_writes: true\n", "names a worklist"},
		"no criterion":              {strings.Replace(goodScenario, "criteria: [Names the evidence.]\n", "", 1), "at least one criterion"},
		"an empty criterion":        {strings.Replace(goodScenario, "[Names the evidence.]", "[\"\"]", 1), "none may be empty"},
		"an unknown key":            {goodScenario + "worklst: [x]\n", "field worklst not found"},
		"yaml that does not decode": {"name: [", "agentloope2e"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadScenarios(plant(t, map[string]string{"s.yaml": tc.body}))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestTwoScenariosMayNotShareAName(t *testing.T) {
	_, err := LoadScenarios(plant(t, map[string]string{"a.yaml": goodScenario, "b.yaml": goodScenario}))
	if err == nil || !strings.Contains(err.Error(), "two scenarios are named annotates") {
		t.Errorf("err = %v", err)
	}
}
