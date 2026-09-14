// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A scenario must certify the path production actually takes.
//
// The verdict engine answers from the ADDRESS first: a role mailbox is settled
// by mailrole with no model call at all. A corpus case whose address that list
// already answers therefore measures a model on a message production would
// never show it, and can be certified green while the shipped behaviour differs.
//
// Only a DISAGREEMENT is a defect. A role-mailbox scenario at a role-mailbox
// address certifies the answer production gives anyway, and is redundant rather
// than wrong.
//
// The clinic case is why this exists. It was authored at `rezeption@`, expecting
// the model to weigh whose matter the appointment was — and `rezeption` is in
// the role vocabulary, so production settled it as a role mailbox before the
// prompt was built. The census also found a spam scenario with the same shape,
// which is a real disagreement and older than this change.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/modules/capture"
)

func TestNoVerdictScenarioCertifiesAnAddressTheListAlreadyAnswers(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("aicert", "corpus", "capture_counterparty_verdict")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the verdict corpus: %v", err)
	}
	checked := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		var scenario struct {
			Fixture struct {
				Email string `yaml:"email"`
			} `yaml:"fixture"`
			Expect struct {
				Answer string `yaml:"answer"`
			} `yaml:"expect"`
		}
		if err := yaml.Unmarshal(raw, &scenario); err != nil {
			t.Fatalf("parsing %s: %v", entry.Name(), err)
		}
		if scenario.Fixture.Email == "" {
			continue
		}
		checked++
		// Only where the two would DISAGREE. A role-mailbox scenario whose
		// address the list also calls a role mailbox certifies the same answer
		// production gives, so the shortcut costs nothing — the case is
		// redundant rather than wrong. A scenario expecting anything else is
		// measuring a judgment that never happens.
		if addressIsARoleMailbox(scenario.Fixture.Email) && scenario.Expect.Answer != capture.KindRoleMailbox {
			t.Errorf("%s certifies %s as %q, but the role list settles that address as a "+
				"role mailbox before the model is asked — production and this scenario "+
				"give different answers, and only the scenario is measured",
				entry.Name(), scenario.Fixture.Email, scenario.Expect.Answer)
		}
	}
	// A census that read nothing reports the same silence as one that found
	// nothing wrong.
	if checked == 0 {
		t.Fatal("no verdict scenarios were read — this census found no corpus to check")
	}
}
