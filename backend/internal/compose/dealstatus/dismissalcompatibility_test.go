// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestGeneratedTaskSubjectsKeepExistingDismissalDigests(t *testing.T) {
	deal := openDeal()
	deal.Id = openapi_types.UUID(ids.MustParse("00000000-0000-7000-8000-000000000001"))
	deal.Name = "Stable deal"
	last := act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -10))
	last.Id = openapi_types.UUID(ids.MustParse("00000000-0000-7000-8000-000000000002"))
	contact := Seat{Role: roleChampion, Name: "Stable name", Attachable: true, ContactID: ids.MustParse("00000000-0000-7000-8000-000000000003")}
	roleOnly := contact
	roleOnly.Attachable = false
	request := inboundMail(testNow.AddDate(0, 0, -3))
	request.Id = openapi_types.UUID(ids.MustParse("00000000-0000-7000-8000-000000000004"))
	unnamed := request
	unnamed.Subject = nil
	for _, test := range []struct {
		name  string
		facts facts
		want  string
	}{
		{"quiet", facts{deal: deal, now: testNow, loc: time.UTC, timeline: []crmcontracts.Activity{last}}, "7d95ae7d314ed00a827ea001c173b12450c58ad0492adbb062db0be0a269fbc8"},
		{"named", facts{deal: deal, now: testNow, loc: time.UTC, timeline: []crmcontracts.Activity{last}, seats: []Seat{contact}}, "5e094cc191b3c5f8ba67e12352fefec4b486e5a690c9f3b3a59106b38869434b"},
		{"role", facts{deal: deal, now: testNow, loc: time.UTC, timeline: []crmcontracts.Activity{last}, seats: []Seat{roleOnly}}, "606b4deaa778fc3c41863b86ad104ea1f8fc3b3fd6dfbc87aa27a19538df2cb8"},
		{"request", facts{deal: deal, now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{request}}, "3911134405d4f6b0e93de3ccac633dbca5266fbe5e625e6e2f7b3f155235da09"},
		{"unnamed", facts{deal: deal, now: testNow, loc: time.UTC, requests: []crmcontracts.Activity{unnamed}}, "2130efbcb1afd0b72cfa0a031a3da22a71581785f38f615bd9b2044a54d64ab2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, language := range textlang.Shipped {
				test.facts.lang = string(language)
				move := decideMove(test.facts)
				if move.Fingerprint == nil || *move.Fingerprint != test.want {
					t.Fatalf("%s dismissal = %s, want %s", language, fingerprintOf(move), test.want)
				}
			}
		})
	}
}
