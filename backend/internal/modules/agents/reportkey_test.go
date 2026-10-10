// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A report key outside the catalog is an argument the caller got wrong. Reading
// it as a missing record sends an agent hunting for a record, while the list of
// keys that would have worked sat behind the misleading sentence.
func TestAnUnknownReportKeyIsRefusedAsTheArgumentItIs(t *testing.T) {
	d := idProbeDispatcher(t)
	ctx := scopedAgentCtx(principal.ScopeRead)

	_, err := d.registry.Invoke(ctx, "run_report", json.RawMessage(`{"report":"zz_not_a_report"}`))

	var badArgs *BadArgsError
	if !errors.As(err, &badArgs) {
		t.Fatalf("an unknown report key answered %T (%v), want *BadArgsError", err, err)
	}
	if badArgs.Field != "report" {
		t.Errorf("refusal names %q as the field to fix, want report", badArgs.Field)
	}
	said := d.explain("run_report", err)
	if strings.Contains(said, "No such record") {
		t.Errorf("the agent is told a record is missing: %q", said)
	}
	for _, entry := range probeReportCatalog {
		if !strings.Contains(said, entry.Report) {
			t.Errorf("the refusal does not list %q among the keys that would have worked: %q", entry.Report, said)
		}
	}
}

// A key the catalog serves is not caught by the check, whatever else the call
// gets wrong.
func TestAServedReportKeyReachesTheEngine(t *testing.T) {
	d := idProbeDispatcher(t)
	ctx := scopedAgentCtx(principal.ScopeRead)

	_, err := d.registry.Invoke(ctx, "run_report", json.RawMessage(`{"report":"`+probeReportCatalog[0].Report+`"}`))

	if !errors.Is(err, errSeamReached) {
		t.Errorf("a served report key never reached the engine: %v", err)
	}
}
