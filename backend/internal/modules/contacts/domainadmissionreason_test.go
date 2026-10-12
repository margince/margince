// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A row this build has no word for travels as unknown, never as somebody's
// words. A machine sentence sent as `stated` reads as a colleague's reason.
func TestAReasonNoWriterSpellsTravelsAsUnknown(t *testing.T) {
	cases := map[string]BlockedDomain{
		"a heuristic refusal":     {Admission: DomainSuppressed, Source: AdmissionSourceHeuristic, Reason: "looked like a list"},
		"an unknown open reason":  {Admission: DomainUndecided, Source: "renamed_reason"},
		"a human decision unsaid": {Admission: DomainAdmitted, Source: AdmissionSourceHuman},
		"an unknown source":       {Admission: DomainSuppressed, Source: "importer", Reason: "from the old CRM"},
	}
	for name, entry := range cases {
		code, note, kind := reasonCodeOf(entry)
		if code != crmcontracts.BlockedDomainReasonCodeUnknown || note != nil || kind != nil {
			t.Errorf("%s = %q, note %v, sender kind %v, want unknown with neither", name, code, note, kind)
		}
	}
}
