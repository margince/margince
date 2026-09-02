// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The record-history read (GET /records/{entity_type}/{id}/history): one
// plain-language line per audit row — the whole-mutation view next to
// field-history's per-field diffs. Gated exactly like every other record
// read (human-only, object-read, row-scope), keyset-paginated ASC, and cut
// at the erasure boundary — but tombstone-INCLUSIVE: the erase line is the
// honest disclosure that the record was scrubbed.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedRecordAuditRow inserts a raw person audit row with full control
// over the actor columns — the record-history read renders actor_id and
// on_behalf_of, which seedAuditActionRow (fieldhistory suite) pins to a
// fixed literal. This suite exercises entity-type dispatch through the
// shared gate stack (proven per-type by the fieldhistory suite), so its
// seeds stay on person. INSERT is the one verb the append-only trigger
// admits, and the append-only trigger admits it.
func seedRecordAuditRow(t *testing.T, e *Env, action string, personID ids.UUID,
	actorType, actorID string, onBehalfOf *ids.UUID, before, after map[string]any, occurredAt time.Time,
) ids.UUID {
	t.Helper()
	rowID := ids.NewV7()
	ctx := principal.WithWorkspaceID(t.Context(), e.WS)
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO audit_log (id, actor_type, actor_id, on_behalf_of,
			                        action, entity_type, entity_id, before, after, occurred_at)
			 VALUES ($1, $2, $3, $4, $5, 'person', $6, $7, $8, $9)`, rowID, actorType, actorID, onBehalfOf, action, personID, storekit.JSONArg(before), storekit.JSONArg(after), occurredAt)
		return err
	})
	if err != nil {
		t.Fatalf("seed record audit row: %v", err)
	}
	return rowID
}

// seedWorkspaceUser inserts an app_user with a distinct display name so
// the read's name resolution has something real to resolve (the harness
// seeds every rep as "Rep"). Owner connection like the harness itself:
// app_user rows are identity fixtures, not app-role writes.
func seedWorkspaceUser(t *testing.T, e *Env, displayName string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := OwnerConn(t).Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, $3)`, id, id.String()+"@recordhistory.test", displayName); err != nil {
		t.Fatalf("seed app_user %q: %v", displayName, err)
	}
	return id
}

func TestRecordHistoryGatesOnPrincipalPermissionAndVisibility(t *testing.T) {
	e := Setup(t)
	// Captured privately by Rep1: a person is otherwise readable by every
	// seat with the grant, so the out-of-scope assertion needs a private
	// capture to exclude the other caller.
	personID := e.SeedPerson(t, "Gated Subject", &e.Rep1)
	e.MakeCapturePrivate(t, "person", personID, e.Rep1)

	// Rep3 is not the captor: 404, not an empty page — existence-hiding
	// on the row-scope gate like every record read.
	outsider := e.As(e.Rep3, []ids.UUID{e.Team2}, RepPerms)
	if _, err := privacy.ListRecordHistory(outsider, e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID,
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("out-of-scope read: err = %v, want not found", err)
	}

	// A principal without person:read at all: 403 before any row is touched.
	noRead := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{RowScope: principal.RowScopeTeam})
	if _, err := privacy.ListRecordHistory(noRead, e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID,
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("no-permission read: err = %v, want permission denied", err)
	}

	// The history surface is human-only: an agent principal is refused
	// outright, before the entity gate.
	if _, err := privacy.ListRecordHistory(e.AgentCtx(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID,
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("agent-principal read: err = %v, want permission denied", err)
	}
}

func TestRecordHistoryRendersEveryActorChronologically(t *testing.T) {
	e := Setup(t)
	personID := e.SeedPerson(t, "History Subject", nil)
	uma := seedWorkspaceUser(t, e, "Uma Underwriter")
	ada := seedWorkspaceUser(t, e, "Ada Authority")

	// SeedPerson's create row is stamped at real "now"; the four actor
	// rows are dated forward so ordering is unambiguous.
	base := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	seedRecordAuditRow(t, e, "update", personID, "human", "human:"+uma.String(), nil,
		map[string]any{"email": "old@x.com"}, map[string]any{"email": "new@x.com"}, base)
	seedRecordAuditRow(t, e, "update", personID, "agent", "agent:enrich", &ada,
		nil, map[string]any{"title": "CTO"}, base.Add(time.Hour))
	seedRecordAuditRow(t, e, "archive", personID, "system", "system", nil,
		nil, nil, base.Add(2*time.Hour))
	seedRecordAuditRow(t, e, "update", personID, "connector", "connector:hubspot", nil,
		nil, map[string]any{"phone": "1"}, base.Add(3*time.Hour))

	page, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Entries) != 5 {
		t.Fatalf("entries = %d, want 5 (create genesis + four seeded actors): %+v", len(page.Entries), page.Entries)
	}
	if page.HasMore || page.NextCursor != "" {
		t.Errorf("single page must report exhaustion: has_more=%v cursor=%q", page.HasMore, page.NextCursor)
	}
	// Newest first: a record's history answers "what just happened", and the
	// change somebody wants to put back is almost always the last one.
	for i := 1; i < len(page.Entries); i++ {
		if page.Entries[i].OccurredAt.After(page.Entries[i-1].OccurredAt) {
			t.Fatalf("entries not newest-first at index %d: %v before %v",
				i, page.Entries[i].OccurredAt, page.Entries[i-1].OccurredAt)
		}
	}

	// The page is newest first; this suite reads the record's story in the
	// order it happened, so it indexes from the OLD end. Naming the direction
	// once beats flipping five subscripts and getting one of them wrong.
	inStoryOrder := func(i int) privacy.RecordHistoryEntry {
		return page.Entries[len(page.Entries)-1-i]
	}

	// The genesis row's actor is the harness admin, a real app_user whose
	// display name is "Rep" like every harness seat, so the summary resolves
	// the name rather than falling back to the raw prefixed actor_id.
	genesis := inStoryOrder(0)
	if genesis.Action != "create" || genesis.Summary != "Rep created the record" {
		t.Errorf("genesis line = %q (action %q), want the admin's resolved create line", genesis.Summary, genesis.Action)
	}

	human := inStoryOrder(1)
	if human.Summary != "Uma Underwriter updated the record" {
		t.Errorf("human line = %q, want resolved display name", human.Summary)
	}
	// The name is also a FIELD, not only a phrase inside `summary`: a client
	// that renders its own attribution must not have to parse the sentence.
	if human.ActorName == nil || *human.ActorName != "Uma Underwriter" {
		t.Errorf("human line ActorName = %v, want Uma Underwriter", human.ActorName)
	}
	if human.After["email"] != "new@x.com" || human.Before["email"] != "old@x.com" {
		t.Errorf("human line payload = before %v after %v, want the seeded images served", human.Before, human.After)
	}

	agent := inStoryOrder(2)
	// PD-002: the granting human is the SUBJECT of the line and the agent is
	// the qualifier on them, not the other way round.
	if agent.Summary != "Ada Authority, via an agent, updated the record" {
		t.Errorf("agent line = %q, want the granting human named first", agent.Summary)
	}
	// An agent id resolves to no actor name — a machine has none, and its
	// human context is OnBehalfOfName. Never an invented one.
	if agent.ActorName != nil {
		t.Errorf("agent line ActorName = %v, want nil for a machine actor", agent.ActorName)
	}
	if agent.OnBehalfOf == nil || *agent.OnBehalfOf != ada {
		t.Errorf("agent line OnBehalfOf = %v, want %v", agent.OnBehalfOf, ada)
	}
	if agent.OnBehalfOfName == nil || *agent.OnBehalfOfName != "Ada Authority" {
		t.Errorf("agent line OnBehalfOfName = %v, want Ada Authority", agent.OnBehalfOfName)
	}

	if got := inStoryOrder(3).Summary; got != "System archived the record" {
		t.Errorf("system line = %q", got)
	}
	if got := inStoryOrder(4).Summary; got != "Connector updated the record" {
		t.Errorf("connector line = %q", got)
	}
}

// TestRecordHistoryErasureBoundaryServesOnlyTheTombstone proves D1's
// tombstone-INCLUSIVE cut: after the REAL erasure engine runs, every line
// older than the erase tombstone is withheld (their before/after images
// are exactly the PII the scrub certified gone), while the tombstone line
// itself IS served — its images are empty (the suppression tallies ride
// audit_log.evidence, which this read never selects), so the erase line is
// honest disclosure, not a leak.
func TestRecordHistoryErasureBoundaryServesOnlyTheTombstone(t *testing.T) {
	e := Setup(t)
	personID := e.SeedPerson(t, "Selma Subject", nil)
	past := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Microsecond)
	seedRecordAuditRow(t, e, "update", personID, "human", "user-1", nil,
		map[string]any{"email": "selma@example.com"},
		map[string]any{"email": "selma.subject@example.com"}, past)

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), personID, "dsr"); err != nil {
		t.Fatalf("erase: %v", err)
	}

	page, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID,
	})
	if err != nil {
		t.Fatalf("post-erasure list: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("entries = %d, want exactly the tombstone (pre-erasure lines withheld): %+v",
			len(page.Entries), page.Entries)
	}
	tomb := page.Entries[0]
	if tomb.Action != "erase" {
		t.Fatalf("surviving line action = %q, want erase", tomb.Action)
	}
	// The eraser stamps the calling principal — the harness admin is a
	// human — so the line renders from the human branch, honestly.
	if tomb.ActorType != "human" || !strings.HasSuffix(tomb.Summary, "erased the record") {
		t.Errorf("tombstone line = %q (actor %q), want an erased-the-record line by the caller",
			tomb.Summary, tomb.ActorType)
	}
	if len(tomb.Before) != 0 || len(tomb.After) != 0 {
		t.Errorf("tombstone images must be empty (meta rides evidence): before %v after %v",
			tomb.Before, tomb.After)
	}

	// The boundary is a cut, not a ban: a change made AFTER the scrub is
	// ordinary history again, and on a newest-first page it reads BEFORE the
	// erase line.
	future := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	seedRecordAuditRow(t, e, "update", personID, "human", "user-1", nil,
		nil, map[string]any{"owner_id": "rep-2"}, future)
	page, err = privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID,
	})
	if err != nil {
		t.Fatalf("post-scrub list: %v", err)
	}
	if len(page.Entries) != 2 || page.Entries[0].Action != "update" || page.Entries[1].Action != "erase" {
		t.Fatalf("post-scrub timeline = %+v, want [update, erase] newest first", page.Entries)
	}
}

// The keyset walks BACKWARDS in time, one row per page, without serving a row
// twice. Newest first is the order the surface reads, so the walk starts at the
// most recent change and ends at the record's genesis.
func TestRecordHistoryKeysetWalksNewestFirstWithoutOverlap(t *testing.T) {
	e := Setup(t)
	// SeedPerson's create row is the true oldest line and is therefore served
	// LAST; two forward-dated updates make three rows total.
	personID := e.SeedPerson(t, "Paging Subject", nil)
	base := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	r2 := seedRecordAuditRow(t, e, "update", personID, "human", "user-1", nil,
		map[string]any{"phone": "1"}, map[string]any{"phone": "2"}, base)
	r3 := seedRecordAuditRow(t, e, "update", personID, "human", "user-1", nil,
		map[string]any{"phone": "2"}, map[string]any{"phone": "3"}, base.Add(time.Hour))

	one := 1
	var walked []ids.UUID
	var cursor *string
	for pageNo := 1; pageNo <= 3; pageNo++ {
		page, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
			EntityType: "person", EntityID: personID, Limit: &one, Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("page %d: %v", pageNo, err)
		}
		if len(page.Entries) != 1 {
			t.Fatalf("page %d entries = %d, want 1", pageNo, len(page.Entries))
		}
		walked = append(walked, page.Entries[0].ID)
		if pageNo < 3 {
			if !page.HasMore || page.NextCursor == "" {
				t.Fatalf("page %d must report more rows follow", pageNo)
			}
			cursor = &page.NextCursor
		} else if page.HasMore || page.NextCursor != "" {
			t.Fatalf("page 3 is genuine exhaustion — has_more must not lie")
		}
	}
	// Newest first: the two forward-dated updates come before the genesis row.
	if walked[0] != r3 || walked[1] != r2 {
		t.Fatalf("walk order = %v, want [%v, %v, genesis]", walked, r3, r2)
	}
	seen := map[ids.UUID]bool{}
	for _, id := range walked {
		if seen[id] {
			t.Fatalf("row %v served on two pages — keyset overlap", id)
		}
		seen[id] = true
	}
}

// TestRecordHistoryHonestEmptyPageBeyondTheFinalRow: every store write
// audits itself, so a VISIBLE record with a truly empty spine cannot be
// seeded honestly through the stores — the honest zero-match construction
// is a cursor positioned past the record's final row: the full gate stack
// still runs, and the scan matches nothing.
func TestRecordHistoryHonestEmptyPageBeyondTheFinalRow(t *testing.T) {
	e := Setup(t)
	personID := e.SeedPerson(t, "Quiet Subject", nil)

	full, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID,
	})
	if err != nil {
		t.Fatalf("full list: %v", err)
	}
	if len(full.Entries) == 0 {
		t.Fatal("SeedPerson must have audited its own create — harness drift")
	}
	last := full.Entries[len(full.Entries)-1]
	pastTheEnd, err := storekit.EncodeCursor(last.OccurredAt, last.ID)
	if err != nil {
		t.Fatalf("minting a cursor past the end: %v", err)
	}

	page, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID, Cursor: &pastTheEnd,
	})
	if err != nil {
		t.Fatalf("empty page must not error: %v", err)
	}
	if page.Entries == nil || len(page.Entries) != 0 || page.HasMore || page.NextCursor != "" {
		t.Fatalf("want honest empty page (non-nil, zero entries, no more): %+v", page)
	}
}

// TestRecordHistoryErasureBoundsCollateralScrubs mirrors the field-history
// sibling (TestFieldHistoryErasureBoundsCollateralScrubs): the eraser's
// reach is entity-generic — it doesn't stop at the person it was called
// on. The lead twin shares the subject's email (the tie the eraser
// follows) and the activity carries the subject's name in its subject
// line; both create images predate the scrub and both records get their
// OWN erase tombstone. Record-history must honor the same
// tombstone-inclusive cut on every collaterally-scrubbed record: every
// pre-erasure line withheld, the erase line itself served with empty
// images, and the pre-erasure email never surfacing in any entry.
func TestRecordHistoryErasureBoundsCollateralScrubs(t *testing.T) {
	e := Setup(t)
	personID := e.SeedPerson(t, "Selma Subject", nil)
	const twinEmail = "selma.twin@example.test"
	// The subject's address is what ties the twin to the person: the
	// eraser wipes any lead carrying one of the subject's emails.
	e.WsExec(t, `INSERT INTO person_email (person_id, email, source, captured_by)
		 VALUES ($1, $2, 'manual', 'human:x')`,
		personID, twinEmail)
	leadID := seedLead(t, e, "Selma Subject", twinEmail, nil)

	activity, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: strPtr("Call with Selma"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: personID}},
	})
	if err != nil {
		t.Fatalf("log activity: %v", err)
	}

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), personID, "dsr"); err != nil {
		t.Fatalf("erase: %v", err)
	}

	targets := []struct {
		entityType string
		id         ids.UUID
	}{
		{"lead", leadID.UUID},
		{"activity", ids.UUID(activity.Id)},
	}
	for _, target := range targets {
		page, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
			EntityType: target.entityType, EntityID: target.id,
		})
		if err != nil {
			t.Fatalf("%s record-history: %v", target.entityType, err)
		}
		// Tombstone-inclusive boundary: exactly the collateral erase line
		// survives, every pre-erasure line (the twin's create, the
		// activity's create) is withheld.
		if len(page.Entries) != 1 || page.HasMore || page.NextCursor != "" {
			t.Fatalf("%s: entries = %d (has_more=%v cursor=%q), want exactly the tombstone: %+v",
				target.entityType, len(page.Entries), page.HasMore, page.NextCursor, page.Entries)
		}
		tomb := page.Entries[0]
		if tomb.Action != "erase" {
			t.Fatalf("%s surviving line action = %q, want erase", target.entityType, tomb.Action)
		}
		if len(tomb.Before) != 0 || len(tomb.After) != 0 {
			t.Errorf("%s tombstone images must be empty (meta rides evidence): before %v after %v",
				target.entityType, tomb.Before, tomb.After)
		}
		// Belt-and-braces on the concrete PII, independent of the entry
		// count above: the pre-erasure email must not surface in ANY
		// entry's actor id, summary, or before/after images.
		for _, entry := range page.Entries {
			if strings.Contains(entry.Summary, twinEmail) || strings.Contains(entry.ActorID, twinEmail) {
				t.Errorf("%s entry %s leaked the erased email in actor/summary: %+v",
					target.entityType, entry.Action, entry)
			}
			for field, images := range map[string]map[string]any{"before": entry.Before, "after": entry.After} {
				for k, v := range images {
					if s, ok := v.(string); ok && strings.Contains(s, twinEmail) {
						t.Errorf("%s entry %s leaked the erased email in %s[%s]: %v",
							target.entityType, entry.Action, field, k, v)
					}
				}
			}
		}
	}
}

func TestRecordHistoryMalformedCursorIsAClientFault(t *testing.T) {
	e := Setup(t)
	personID := e.SeedPerson(t, "Cursor Subject", nil)
	bad := "%%%not-a-cursor"
	_, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID, Cursor: &bad,
	})
	var malformed *storekit.MalformedCursorError
	if !errors.As(err, &malformed) {
		t.Fatalf("err = %v, want *storekit.MalformedCursorError untouched", err)
	}
}

// One recorded event is one read.
//
// The trail is served newest-first and 20 to a page, so a caller after a single
// event — "how was this lead promoted", "when was it erased" — either pages
// until it turns up or reads the first page and gets a confident wrong answer.
// The promoted-lead panel hit exactly that: it reported the outcome as
// unknowable on the leads somebody had worked hardest, because those are the
// ones with enough other rows to push the one it wanted off the page it read
// (issue #1611).
//
// Seeded so the wanted row is NOT the newest and NOT the oldest. A filter that
// silently did nothing would still return it at one of those two ends, and the
// case would pass over a `WHERE` nobody wired.
func TestRecordHistoryAnswersOneVerbWithoutAWalk(t *testing.T) {
	e := Setup(t)
	personID := e.SeedPerson(t, "Filtered Subject", nil)
	actor := seedWorkspaceUser(t, e, "Vera Verb")

	base := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	for i, action := range []string{"update", "assign", "archive", "update", "restore"} {
		seedRecordAuditRow(t, e, action, personID, "human", "human:"+actor.String(), nil,
			nil, map[string]any{"n": i}, base.Add(time.Duration(i)*time.Hour))
	}

	archive := "archive"
	page, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID, Action: &archive,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("entries = %d, want exactly the one archive row — a filter that narrowed "+
			"nothing would answer the whole trail: %+v", len(page.Entries), page.Entries)
	}
	if page.Entries[0].Action != archive {
		t.Errorf("answered a %q row for action=%q", page.Entries[0].Action, archive)
	}
	// And it is the WHOLE answer: a caller reading one verb must be told there
	// is no more, or they page on for rows that do not exist.
	if page.HasMore || page.NextCursor != "" {
		t.Errorf("a filtered page that holds every match must report exhaustion: "+
			"has_more=%v cursor=%q", page.HasMore, page.NextCursor)
	}
}

// A KNOWN verb this record never saw answers an honest empty page.
//
// Named for what it checks, which is the store's half. The other half — a verb
// the installation does not record at all — is refused 422 by the HANDLER, and
// asserting it here would be asserting it of a function that never sees it:
// ListRecordHistory takes a filter, not a request, so a bad verb reaching it
// has already passed the door that was supposed to stop it.
//
// The two answers must stay different. This one is a fact about the record;
// the other is a mistyped request, and a caller who reads an empty page for it
// concludes something false about their data.
func TestRecordHistoryAnswersAnEmptyPageForAVerbThisRecordNeverSaw(t *testing.T) {
	e := Setup(t)
	personID := e.SeedPerson(t, "Refusal Subject", nil)

	// Reaches the store, which is where an empty page would be manufactured.
	// The wire refusal is the handler's and is asserted where the handler is.
	neverHappened := "restore"
	page, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: personID, Action: &neverHappened,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Entries) != 0 {
		t.Errorf("entries = %d, want none — this record has no restore row", len(page.Entries))
	}
}
