// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestEveryProducerStatesAnOwner is the census, and the reason ownerRef carries
// a kind rather than a bare id.
//
// A lane that never answers reaches a reader as a row with no owner, and the
// next thing anybody would do about that is default it — to the reader,
// because the row is on their page, or to `unassigned`, because nothing was
// found. Both defaults are wrong in the same direction and neither fails: the
// row renders, the count adds up, and a rep's morning quietly fills with work
// they do not owe.
//
// The corpus is a day carrying EVERY lane, so a producer added to classifyDay
// without an answer appears here by arriving in the day rather than by somebody
// adding it to a list.
func TestEveryProducerStatesAnOwner(t *testing.T) {
	t.Parallel()
	rows := classifyDay(dayOfEveryLane(), rankInstant, dayMoney{})
	// The two lanes read BESIDE the assembled day rather than as part of it —
	// the who-is-waiting and owed-leads reads take the scope as a query argument
	// — so classifyDay never produces them and a census over it alone would
	// leave both unexamined. They are appended through their own classifiers,
	// which is what the assembler calls.
	rows = append(rows,
		classifyWaiting(WaitingCustomer{Since: rankInstant}, rankInstant),
		classifyLead(OwedLead{}, rankInstant))
	if len(rows) == 0 {
		t.Fatal("the fixture produced no rows, so this census would pass over nothing")
	}
	seen := map[crmcontracts.WorklistItemSource]bool{}
	for _, row := range rows {
		seen[row.item.Source] = true
		if row.ownerRef.kind == ownerUnstated {
			t.Errorf("a %q row reached the page and no producer said who answers for it: "+
				"state one in its classifier — ownedBy, unassigned, ownedByWhoeverIsReading "+
				"or deferredToTheDeal", row.item.Source)
		}
	}
	// And the fixture really did drive every lane. Without this the census
	// passes on a day that produced three sources: a producer added later would
	// be as unexamined as it was before anybody wrote this test.
	for _, source := range ClassifiedSources() {
		if !seen[source] {
			t.Errorf("no %q row reached this census, so nothing here proves that producer "+
				"states an owner: give dayOfEveryLane a row for it", source)
		}
	}
}

// TestAReaderLaneNamesTheReader holds the one answer that needs somebody to
// resolve it.
//
// The per-user lanes say "whoever is reading" rather than a resolved id,
// because the classifiers are pure functions of the day. If the wire step
// dropped the reader the rows would carry no owner at all — which reads to a
// client exactly like a lane that never answered.
func TestAReaderLaneNamesTheReader(t *testing.T) {
	t.Parallel()
	reader := ids.MustParse("01a05500-0000-7000-8000-00000000feed")
	row := ranked{
		item:     crmcontracts.WorklistItem{Source: crmcontracts.WorklistItemSourceNotice},
		ownerRef: ownedByWhoeverIsReading(),
	}

	owner := ownerOnTheWire(row, reader)

	if owner == nil {
		t.Fatal("a personal lane's row carries no owner, which reads as a lane that never answered")
	}
	if owner.Kind != crmcontracts.WorklistOwnerUser {
		t.Errorf("a personal lane's row answers %q, want a named user", owner.Kind)
	}
	if owner.Id == nil || ids.UUID(*owner.Id) != reader {
		t.Errorf("a personal lane's row names %v, want the reader %v", owner.Id, reader)
	}
}

// TestAnAgentGetsNoOwnerRatherThanAnUnassignedClaim.
//
// A call with nothing human behind it has no personal lane. Answering
// `unassigned` there would be a claim about the WORK — nobody owns this — made
// on the basis of a fact about the CALLER, and a manager reading that queue
// would see a rep's own notices reported as unheld.
func TestAnAgentGetsNoOwnerRatherThanAnUnassignedClaim(t *testing.T) {
	t.Parallel()
	row := ranked{
		item:     crmcontracts.WorklistItem{Source: crmcontracts.WorklistItemSourceNotice},
		ownerRef: ownedByWhoeverIsReading(),
	}

	if owner := ownerOnTheWire(row, ids.UUID{}); owner != nil {
		t.Errorf("a reader-bound row answered %+v to a caller with no human behind it, "+
			"want no owner stated", *owner)
	}
}

// TestUnassignedIsSaidRatherThanImplied separates the two things a missing
// owner can mean.
//
// A lane that read an owner column and found none is stating a fact the
// unassigned scope exists to surface. A lane that never answered is silence.
// They must not reach the wire the same way, or the census above could not tell
// them apart either.
func TestUnassignedIsSaidRatherThanImplied(t *testing.T) {
	t.Parallel()
	stated := ranked{
		item:     crmcontracts.WorklistItem{Source: crmcontracts.WorklistItemSourceTask},
		ownerRef: unassigned(),
	}
	silent := ranked{item: crmcontracts.WorklistItem{Source: crmcontracts.WorklistItemSourceTask}}

	said := ownerOnTheWire(stated, ids.UUID{})
	if said == nil || said.Kind != crmcontracts.WorklistOwnerUnassigned {
		t.Errorf("a lane that found no owner answered %v, want an explicit unassigned", said)
	}
	if quiet := ownerOnTheWire(silent, ids.UUID{}); quiet != nil {
		t.Errorf("a lane that never answered claimed %+v, which a reader cannot tell from a "+
			"real unassigned row", *quiet)
	}
}

// TestATasksOwnerAgreesWithItsOwnReason holds the two spellings of one fact
// together.
//
// The task lane says "nobody has taken this" twice — as a reason a reader sees
// and as the owner a client draws. Two spellings that could disagree is how a
// row ends up naming an owner beside the word `unassigned`.
func TestATasksOwnerAgreesWithItsOwnReason(t *testing.T) {
	t.Parallel()
	for _, probe := range []struct {
		name       string
		task       crmcontracts.AttentionItem
		unassigned bool
	}{
		{"nobody has taken it", item("t1", "task"), true},
		{"assigned", assignedTask("t2"), false},
	} {
		t.Run(probe.name, func(t *testing.T) {
			row := classifyTask(probe.task, rankInstant)
			saysUnassigned := false
			for _, because := range row.item.Because {
				if because.Kind == "unassigned" {
					saysUnassigned = true
				}
			}
			owner := ownerOnTheWire(row, ids.UUID{})
			ownerSaysUnassigned := owner != nil && owner.Kind == crmcontracts.WorklistOwnerUnassigned
			if saysUnassigned != ownerSaysUnassigned {
				t.Errorf("the row's reasons say unassigned=%v and its owner says %v: "+
					"a reader is told two different things about one fact",
					saysUnassigned, ownerSaysUnassigned)
			}
			if saysUnassigned != probe.unassigned {
				t.Errorf("the row reads unassigned=%v, want %v", saysUnassigned, probe.unassigned)
			}
		})
	}
}

// assignedTask is a task somebody holds.
func assignedTask(id string) crmcontracts.AttentionItem {
	at := item(id, "task")
	assignee := openapi_types.UUID(ids.MustParse("01a05500-0000-7000-8000-00000000a551"))
	at.AssigneeId = &assignee
	return at
}

// dayOfEveryLane drives every producer classifyDay knows, so the census reads
// the whole set rather than the lanes somebody remembered.
func dayOfEveryLane() crmcontracts.Attention {
	return crmcontracts.Attention{
		AsOf:              rankInstant,
		Meetings:          lane(item("meeting", "meeting")),
		ThisMorning:       []crmcontracts.AttentionItem{item("brief", "brief_item")},
		Commitments:       lane(item("promise", "conversation_claim")),
		DidNotRun:         lane(item("failed", "failed_approval")),
		Dsr:               lane(item("dsr", "dsr")),
		AtRisk:            lane(item("deal", "deal_at_risk")),
		Planned:           []crmcontracts.AttentionItem{item("task", "task")},
		Bounces:           lane(item("bounce", "bounce")),
		Undelivered:       lane(item("undelivered", "undelivered")),
		NeedsYou:          []crmcontracts.AttentionItem{item("approval", "approval"), item("pair", "dedupe_candidate")},
		RelationshipDecay: lane(item("decay", "relationship_decay")),
		CaptureHealth:     lane(item("capture", "capture_health")),
		AiWorkHealth:      lane(item("ai", "ai_work_health")),
		AutomationHealth:  lane(item("automation", "automation_run")),
		SyncHealth:        lane(item("sync", "sync_health")),
		Notices:           lane(item("notice", "notice")),
		Introductions:     lane(item("introduction", "introduction_request")),
	}
}
