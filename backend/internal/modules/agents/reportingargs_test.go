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

// A read_reporting call that gets one argument wrong is told which one and what
// would have worked, not "invalid argument".
func TestAReadReportingMistakeNamesTheArgumentAndTheFix(t *testing.T) {
	registry := idProbeDispatcher(t).registry
	ctx := scopedAgentCtx(principal.ScopeRead)

	for _, tc := range []struct {
		name, args, field, says string
	}{
		{"a mode that is not one", `{"mode":"get"}`, "mode", "catalog, evaluate, reports"},
		{"no mode at all", `{}`, "", "`mode` is missing"},
		{"a report read with no id", `{"mode":"report"}`, "id", "required in mode report"},
		{"editions with no report id", `{"mode":"editions"}`, "id", "required in mode editions"},
		{"a comparison with one side", `{"mode":"compare","id":"0192a5c0-0000-7000-8000-000000000001"}`, "right_id", "required in mode compare"},
		{"a page size above the ceiling", `{"mode":"catalog","limit":101}`, "", "above its declared maximum of 100"},
		{"a negative page size", `{"mode":"catalog","limit":-1}`, "", "below its declared minimum of 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := registry.Invoke(ctx, "read_reporting", json.RawMessage(tc.args))

			var badArgs *BadArgsError
			if !errors.As(err, &badArgs) {
				t.Fatalf("answered %T (%v), want *BadArgsError", err, err)
			}
			if badArgs.Field != tc.field || !strings.Contains(badArgs.Error(), tc.says) {
				t.Errorf("refusal names field %q with %q, want field %q saying %q",
					badArgs.Field, badArgs.Error(), tc.field, tc.says)
			}
		})
	}
}
