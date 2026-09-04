// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Every verb any lane advertises is one this product can actually perform.
//
// A row's `actions` is a promise to a client: a generated client is entitled to
// draw a control for whatever the server says it offers. When the two disagree
// there is no error anywhere — the server sends a verb, the client has no route
// for it, and the row simply renders with nothing to press. A rep is shown work
// and given no way to do it, which is the one thing this queue exists not to do.
//
// That defect shipped. `brief_item` reached a seller's screen advertising `act`,
// `set_aside` and `dismiss`; the client drew a Pin button and nothing else.
//
// The gate beside this one, TestNoOptionalLaneOffersAnActionTheSurfaceCannotPerform,
// asks the same question of FOUR lanes it names. `brief_item` is not among them,
// which is why the defect was invisible to it. This one derives the lanes from
// the assembled answer itself, so a lane joins the census by existing rather
// than by somebody remembering to add it.

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// performedBySource is what the CLIENT can actually do with each verb, per
// source, and it is a claim about frontend/src/screens/worklist.row.tsx.
//
// Keyed by source rather than by verb alone, because the endpoint a verb posts
// to depends on the row it sits on: `dismiss` is the person's nudge dismissal
// on a decay row and the brief's own mark on a brief item, and the client
// dispatches on `item.source` for exactly that reason. A verb-only entry would
// admit a source the client has no route for — which is the shape of the defect
// this gate exists to catch, so it must not be the shape of the gate.
var performedBySource = map[string][]crmcontracts.AttentionItemActions{
	// Routed to the record the row is about, through VERB_DESTINATION.
	"customer_waiting":   {"open"},
	"lead_response":      {"open"},
	"deal_at_risk":       {"open"},
	"conversation_claim": {"open"},
	"meeting":            {"open"},
	// The task's own verbs. `complete` acts in place through TaskComplete;
	// `snooze` opens the record, where the due date lives.
	"task": {"complete", "snooze", "open"},
	// Answered inline: the decision card is on the row itself.
	"approval":             {"decide", "open"},
	"dedupe_candidate":     {"merge", "open"},
	"introduction_request": {"decide", "open"},
	// Drawn by NoticeAcknowledge rather than through the routing table.
	"notice": {"acknowledge", "open"},
	// The briefing queue's three, posted to /brief/items/{id}/… by BriefVerbs.
	"brief_item": {"act", "set_aside", "dismiss", "open"},
	// The person's nudge dismissal, drawn by NudgeDismiss.
	"relationship_decay": {"dismiss", "open"},
	// Health and delivery rows navigate and nothing more: what fixes them lives
	// on another screen, and a verb here would promise a repair this queue
	// cannot make.
	"sync_health":     {"open"},
	"capture_health":  {"open"},
	"ai_work_health":  {"open"},
	"automation_run":  {"open"},
	"bounce":          {"open"},
	"undelivered":     {"open"},
	"failed_approval": {"open"},
	// The privacy queue's own row. It is read here and answered there.
	"dsr": {"open"},
}

func TestNoLaneAdvertisesAVerbTheClientCannotPerform(t *testing.T) {
	assembled := aDayWithEveryLaneCarryingARow(t)
	lanes := lanesOf(t, assembled)
	// THE SOURCES THAT CARRY VERBS ARE NAMED, and the guard is that they were
	// all reached — not merely that some number of lanes came back.
	//
	// A count is too loose to be a guard here, and that is measured rather than
	// assumed: breaking the reflection walk's slice arm drops the census from
	// 16 lanes to 12 and from 12 verbs to 7, losing exactly `task`,
	// `brief_item`, `approval` and `dedupe_candidate` — the four sources with
	// the most verbs to get wrong, one of which shipped the defect this census
	// exists to catch. A `len(lanes) >= 10` check passes happily through that.
	//
	// So the four are asked for by name. They live on the always-present lanes
	// (Planned, ThisMorning, NeedsYou), which is why a walk that reads only
	// pointer lanes loses them, and naming them is what makes that loss fail.
	// The list spans BOTH shapes the walk reads. `task`, `brief_item`,
	// `approval` and `dedupe_candidate` ride the always-present slice lanes;
	// `relationship_decay` and `notice` ride optional pointer ones. Naming only
	// the first four left the pointer arm free to break in silence, taking
	// twelve lanes with it and still passing.
	mustReach := []string{
		"task", "brief_item", "approval", "dedupe_candidate",
		"relationship_decay", "notice",
	}
	reached := map[string]bool{}
	for _, items := range lanes {
		for _, item := range items {
			reached[string(item.Source)] = true
		}
	}
	for _, source := range mustReach {
		if !reached[source] {
			t.Fatalf("the assembled day carried no %q row, so this census did not look at the "+
				"source's verbs at all: either the fixture stopped feeding that lane or the "+
				"walk stopped reading it, and both leave the census passing over the sources "+
				"with the most verbs to get wrong", source)
		}
	}
	checked := 0
	for lane, items := range lanes {
		for _, item := range items {
			source := string(item.Source)
			allowed, known := performedBySource[source]
			if !known {
				t.Errorf("lane %q carries source %q, which this census does not describe: "+
					"say which verbs the client performs for it in performedBySource, "+
					"or the next verb it grows reaches a reader with nothing to press",
					lane, source)
				continue
			}
			for _, action := range item.Actions {
				checked++
				if !slicesContain(allowed, action) {
					t.Errorf("source %q advertises %q and the client performs %v: "+
						"the row reaches a reader who is shown work and given no way to do it. "+
						"Wire the verb in frontend/src/screens/worklist.row.tsx, or stop sending it",
						source, action, allowed)
				}
			}
		}
	}
	// And the fixture has to actually carry verbs. A day whose lanes all came
	// back empty would satisfy every assertion above without looking at one.
	if checked == 0 {
		t.Fatal("no lane advertised a single verb: the fixture carries no rows, so this census proved nothing")
	}
}

// lanesOf reads every AttentionItem lane off the assembled day by reflection.
//
// Derived rather than named, and that is the whole difference between this
// census and the four-lane check beside it: the lane carrying the defect that
// shipped was simply not on that list, and no failure said so. A lane added
// tomorrow is read here because it exists.
func lanesOf(t *testing.T, day crmcontracts.Attention) map[string][]crmcontracts.AttentionItem {
	t.Helper()
	out := map[string][]crmcontracts.AttentionItem{}
	value := reflect.ValueOf(day)
	structure := value.Type()
	for at := range structure.NumField() {
		field := value.Field(at)
		name := structure.Field(at).Name
		switch field.Kind() {
		case reflect.Slice:
			if items, ok := field.Interface().([]crmcontracts.AttentionItem); ok {
				out[name] = items
			}
		case reflect.Pointer:
			if field.IsNil() {
				continue
			}
			if items, ok := field.Elem().Interface().([]crmcontracts.AttentionItem); ok {
				out[name] = items
			}
		}
	}
	return out
}

func slicesContain(in []crmcontracts.AttentionItemActions, want crmcontracts.AttentionItemActions) bool {
	for _, one := range in {
		if one == want {
			return true
		}
	}
	return false
}

// TestTheCensusDescribesEverySourceTheQueueClassifies keeps the table honest in
// the other direction.
//
// A source the queue draws and this census does not describe is a source whose
// verbs nobody is checking — and because the check above only fires on a lane
// the fixture happens to fill, its absence would be silent.
func TestTheCensusDescribesEverySourceTheQueueClassifies(t *testing.T) {
	described := make([]string, 0, len(performedBySource))
	for source := range performedBySource {
		described = append(described, source)
	}
	sort.Strings(described)
	for _, source := range ClassifiedSources() {
		if _, known := performedBySource[string(source)]; !known {
			t.Errorf("the queue classifies %q and performedBySource does not describe it: "+
				"add the verbs the client performs for that source. Described today: %s",
				source, strings.Join(described, ", "))
		}
	}
	// And nothing described that the queue no longer draws, which would read to
	// the next author as though that source still existed.
	for _, source := range described {
		found := false
		for _, classified := range ClassifiedSources() {
			if string(classified) == source {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("performedBySource describes %q, which the queue no longer classifies", source)
		}
	}
}

// aDayWithEveryLaneCarryingARow assembles one day with every optional lane fed.
//
// A lane the fixture leaves empty is a lane the census silently does not check,
// so every stub carries a row.
func aDayWithEveryLaneCarryingARow(t *testing.T) crmcontracts.Attention {
	t.Helper()
	svc := NewService(
		// The four lanes the sibling check leaves empty, filled here on purpose:
		// a lane the fixture does not feed is a lane this census silently does
		// not read, and `brief_item` — the source that actually shipped a row
		// with nothing to press — is one of them.
		stubApprovals{rows: []crmcontracts.Approval{approval("a staged send")}},
		stubDuplicates{pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "person", Confidence: 0.9,
			LeftID: ids.NewV7(), RightID: ids.NewV7(),
		}}},
		&stubTasks{rows: []Task{{
			ID: ids.NewV7(), Subject: "send the quote",
			LinkType: "person", LinkID: ids.NewV7(),
		}}},
		stubReceipts{},
		stubBriefing{rows: []BriefEntry{{ID: ids.NewV7(), DealID: ids.NewV7(), Rank: 1}}},
		&stubCommitments{rows: []Commitment{promise("a promise", readInstant)}},
		stubAtRisk{rows: []RiskyDeal{{Name: "a deal", QuietDays: 20}}},
		&stubDecay{rows: []QuietRelationship{{Name: "a contact", QuietDays: 63, LastAt: readInstant}}},
		&stubMeetings{rows: []Meeting{{Subject: "a meeting", StartsAt: readInstant}}},
		&stubFailedEffects{rows: []FailedEffect{{
			ID: ids.NewV7(), Kind: "send_email",
			Sentence: "this was approved, but the work it released did not run",
			FailedAt: readInstant, TargetType: "person", TargetID: ids.NewV7(),
		}}},
		&stubDSRs{rows: []DSRCase{{ID: ids.NewV7(), Kind: "access", DueAt: readInstant}}},
		&stubSyncHealth{rows: []SyncConcern{{Kind: "sync_failing", ErrorClass: "auth"}}},
		&stubCaptureHealth{rows: []CaptureConcern{{ConnectionID: ids.NewV7(), Kind: "reauth_required", Provider: "gmail"}}},
		&stubAIWork{rows: []TroubledRun{{ID: ids.NewV7(), State: "failed", OccurredAt: readInstant}}},
		&stubBounces{rows: []BouncedSend{{ID: ids.NewV7(), Subject: "a bounced send", BouncedAt: readInstant, PersonID: ids.NewV7()}}},
		&stubAutomations{rows: []TroubledAutomationRun{{ID: ids.NewV7(), Name: "a broken rule", Outcome: "failed", OccurredAt: readInstant}}},
		&stubNotices{rows: []UnreadNotice{{ID: ids.NewV7(), Kind: "automation", Subject: "a notice", CreatedAt: readInstant}}},
		nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling the day: %v", err)
	}
	return out
}
