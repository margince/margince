// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whose page a meeting's brief is read on.
//
// The links come back already scoped, so this reads what the caller may see and
// nothing else. Its whole job is to pick ONE of them deterministically, because
// the row draws one link and a choice that moved between reads would move a
// control under the reader.

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheBriefOpensOnAPersonTheMeetingNames(t *testing.T) {
	person := ids.NewV7()
	got := personOnMeeting(withLinks(
		link("organization", ids.NewV7()),
		link("person", person),
	))
	if got != person {
		t.Errorf("personOnMeeting = %v, want the person link %v", got, person)
	}
}

// The FIRST person link, in the row's own order.
//
// A meeting with several attendees has several honest answers and the row draws
// one. Taking the store's order makes two reads of an unchanged meeting choose
// the same page; anything cleverer would be a ranking this lane has no basis
// for, and anything unstable moves a control under the reader between reads.
func TestSeveralAttendeesResolveToTheSamePageEveryRead(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()
	row := withLinks(link("person", first), link("person", second))
	for range 3 {
		if got := personOnMeeting(row); got != first {
			t.Fatalf("personOnMeeting = %v, want the first person link %v", got, first)
		}
	}
}

// A meeting naming no person at all yields nothing, and the row then offers no
// brief. An internal meeting is the honest common case; a meeting whose
// attendees the caller may not read arrives the same way, because the links are
// already scoped. Both mean there is no page to read the brief on.
func TestAMeetingLinkingNoPersonNamesNobody(t *testing.T) {
	if got := personOnMeeting(withLinks(link("deal", ids.NewV7()))); !got.IsZero() {
		t.Errorf("personOnMeeting = %v, want nobody", got)
	}
	if got := personOnMeeting(crmcontracts.Activity{}); !got.IsZero() {
		t.Errorf("personOnMeeting(no links at all) = %v, want nobody", got)
	}
}

func withLinks(links ...crmcontracts.ActivityLink) crmcontracts.Activity {
	return crmcontracts.Activity{Links: &links}
}

func link(kind string, id ids.UUID) crmcontracts.ActivityLink {
	return crmcontracts.ActivityLink{
		EntityType: crmcontracts.ActivityLinkEntityType(kind),
		EntityId:   openapi_types.UUID(id),
	}
}
