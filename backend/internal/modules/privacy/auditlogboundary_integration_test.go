// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// The compliance log stops at a scrub the way the two history projections do.
//
// audit_log is append-only — trg_audit_no_mutate refuses an UPDATE and a DELETE
// — so an Art. 17 erase cannot rewrite the images it certifies gone. Every read
// of the spine enforces that boundary itself, or the erase is only as complete
// as the reads that happen to respect it.
//
// ListFieldHistory and ListRecordHistory each take one boundary because each
// reads one record. This read does not: entity_type and entity_id are optional
// filters, so a page can span records that were erased at different moments and
// records that were never erased at all. The boundary is therefore per ROW.
//
// The ROW survives either way. A compliance log answers who did what and when,
// and a write that happened is a fact an erasure does not undo — it is the
// IMAGES that carry what the subject typed, and only those are withheld.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type auditBoundaryEnv struct {
	ctx    context.Context
	db     *database.DB
	person ids.PersonID
	owner  *pgx.Conn
	ws     ids.UUID
	user   ids.UUID
	// other is a SECOND seat, the one who captured the held mail in the
	// audience tests next door. The audience arm admits a row's own capturer
	// (`captured_by LIKE '%:<uuid>'`), so a fixture where the reader captured
	// the activity would pass whatever the redaction did.
	other ids.UUID
}

func setupAuditBoundary(t *testing.T) *auditBoundaryEnv {
	t.Helper()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}

	ws, user, person := ids.NewV7(), ids.NewV7(), ids.New[ids.PersonKind]()
	other := ids.NewV7()
	for _, seed := range []struct {
		statement string
		args      []any
	}{
		{`INSERT INTO workspace (id) VALUES ($1)`, []any{ws}},
		{
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
			[]any{user, "admin-" + user.String() + "@boundary.test"},
		},
		{
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
			[]any{other, "other-" + other.String() + "@boundary.test"},
		},
		{`INSERT INTO person (id, full_name, source, captured_by)
		  VALUES ($1, 'Sara Subject', 'manual', 'user:'||$2::text)`, []any{person, user}},
	} {
		if _, err := owner.Exec(ctx, seed.statement, seed.args...); err != nil {
			t.Fatal(err)
		}
	}

	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })

	return &auditBoundaryEnv{
		ctx:    exportContext(ws, user),
		db:     database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)),
		person: person,
		owner:  owner,
		ws:     ws,
		user:   user,
		other:  other,
	}
}

// auditRow writes one spine row directly. The writers are not the subject here —
// what this read does with a row that already exists is — and seeding through
// them would need an erasure run per case.
func (e *auditBoundaryEnv) auditRow(t *testing.T, action string, at time.Time, before, after string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before, after, occurred_at)
		 VALUES ($1, 'human', 'user:'||$2::text, $3, 'person', $4, $5::jsonb, $6::jsonb, $7)`,
		id, e.user, action, e.person, nullableJSON(before), nullableJSON(after), at); err != nil {
		t.Fatalf("seeding a %s row: %v", action, err)
	}
	return id
}

// nullableJSON renders an empty fixture image as SQL NULL. A scrub tombstone
// carries no images at all, and writing "" would make it an empty one — a
// different answer, and the one this boundary is about telling apart.
// agentRow is one line written by an agent acting for a human: the passport it
// carried and the person behind it are separate columns, and both widen from a
// nullable scan into their own id kind.
func (e *auditBoundaryEnv) agentRow(t *testing.T, at time.Time) (id, passport, onBehalfOf ids.UUID) {
	t.Helper()
	id, passport, onBehalfOf = ids.NewV7(), ids.NewV7(), e.user
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO audit_log (id, actor_type, actor_id, passport_id, on_behalf_of,
		                        action, entity_type, entity_id, before, after, occurred_at)
		 VALUES ($1, 'agent', 'agent:enrich', $2, $3, 'update', 'person', $4,
		         '{"title":"Engineer"}'::jsonb, '{"title":"Architect"}'::jsonb, $5)`,
		id, passport, onBehalfOf, e.person, at); err != nil {
		t.Fatalf("seeding an agent row: %v", err)
	}
	return id, passport, onBehalfOf
}

func nullableJSON(raw string) *string {
	if raw == "" {
		return nil
	}
	return &raw
}

// filtered runs the read the way a compliance question actually arrives — by
// actor, by verb, by window — because the boundary has to hold on the narrowed
// page too. A rule that only applies to the unfiltered read is one a filter
// walks around.
func (e *auditBoundaryEnv) filtered(t *testing.T, f AuditFilter) []AuditEntry {
	t.Helper()
	limit := 50
	f.Limit = &limit
	page, err := ListAuditLog(e.ctx, e.db, f)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	return page.Entries
}

func (e *auditBoundaryEnv) entries(t *testing.T) []AuditEntry {
	t.Helper()
	entityType, entityID, limit := "person", e.person.UUID, 50
	page, err := ListAuditLog(e.ctx, e.db, AuditFilter{
		EntityType: &entityType, EntityID: &entityID, Limit: &limit,
	})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	return page.Entries
}

// imageOf reports what a caller can read out of one row's before image.
func imageOf(t *testing.T, entries []AuditEntry, id ids.UUID) (before, after string) {
	t.Helper()
	for _, e := range entries {
		if e.ID == id {
			return string(e.Before), string(e.After)
		}
	}
	t.Fatalf("the row %s is absent from the page: a compliance log withholds what a write CARRIED, never that it happened", id)
	return "", ""
}

var (
	boundaryEarlier = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	boundaryErasure = time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	boundaryLater   = time.Date(2026, 3, 3, 9, 0, 0, 0, time.UTC)
)

// The disclosure this closes: a name the subject typed, recorded before the
// erasure that deleted it, read back out of the spine afterwards.
func TestAnImageWrittenBeforeAnErasureIsWithheld(t *testing.T) {
	e := setupAuditBoundary(t)
	edit := e.auditRow(t, "update", boundaryEarlier,
		`{"full_name":"Sara Subject"}`, `{"full_name":"Sara Renamed"}`)
	e.auditRow(t, "erase", boundaryErasure, "", "")

	before, after := imageOf(t, e.entries(t), edit)
	if strings.Contains(before, "Sara Subject") || strings.Contains(after, "Sara Renamed") {
		t.Errorf("an erased subject's name is still readable: before=%s after=%s", before, after)
	}
}

// The mirror, and the reason this is a withholding rather than a filter: the row
// itself stays, because a compliance log that hid the write would answer "did
// anybody change this record" with a lie.
func TestTheWriteItselfSurvivesTheErasure(t *testing.T) {
	e := setupAuditBoundary(t)
	edit := e.auditRow(t, "update", boundaryEarlier,
		`{"full_name":"Sara Subject"}`, `{"full_name":"Sara Renamed"}`)
	e.auditRow(t, "erase", boundaryErasure, "", "")

	for _, entry := range e.entries(t) {
		if entry.ID == edit {
			if entry.Action != "update" || entry.ActorID == "" || entry.OccurredAt.IsZero() {
				t.Errorf("the row lost what it is for: %+v", entry)
			}
			return
		}
	}
	t.Error("the erased record's edit vanished from the compliance log entirely")
}

// A write AFTER the scrub is about data the erasure did not touch, so it reads
// whole. Without this the rule would be satisfied by withholding everything.
func TestAnImageWrittenAfterAnErasureIsReadable(t *testing.T) {
	e := setupAuditBoundary(t)
	e.auditRow(t, "erase", boundaryErasure, "", "")
	later := e.auditRow(t, "update", boundaryLater,
		`{"title":"Engineer"}`, `{"title":"Architect"}`)

	before, after := imageOf(t, e.entries(t), later)
	if !strings.Contains(before, "Engineer") || !strings.Contains(after, "Architect") {
		t.Errorf("a post-erasure image was withheld: before=%s after=%s", before, after)
	}
}

// A record nobody erased is untouched by any of this.
func TestARecordWithNoScrubKeepsEveryImage(t *testing.T) {
	e := setupAuditBoundary(t)
	edit := e.auditRow(t, "update", boundaryEarlier,
		`{"full_name":"Sara Subject"}`, `{"full_name":"Sara Renamed"}`)

	before, after := imageOf(t, e.entries(t), edit)
	if !strings.Contains(before, "Sara Subject") || !strings.Contains(after, "Sara Renamed") {
		t.Errorf("an unscrubbed record lost its images: before=%s after=%s", before, after)
	}
}

// The three verbs that certify a scrub, each of which the two history
// projections already stop at. A restriction redacts the identifiers that ARE
// the subject, so images captured before it are as gone to a reader as an
// erase's.
func TestEveryScrubVerbMovesTheBoundary(t *testing.T) {
	for _, verb := range []string{"erase", "anonymize", "restrict"} {
		t.Run(verb, func(t *testing.T) {
			// A fixture of its own: each case writes a scrub row for the SAME
			// subject, and the assertion is that the pre-scrub image is no
			// longer readable. Shared, the second case would be reading an
			// image the first case had already put behind a boundary — green
			// for a verb that does nothing.
			e := setupAuditBoundary(t)
			edit := e.auditRow(t, "update", boundaryEarlier,
				`{"full_name":"Sara Subject"}`, `{"full_name":"Sara Renamed"}`)
			e.auditRow(t, verb, boundaryErasure, "", "")

			before, _ := imageOf(t, e.entries(t), edit)
			if strings.Contains(before, "Sara Subject") {
				t.Errorf("%s left the pre-scrub image readable: %s", verb, before)
			}
		})
	}
}

// The envelope a machine write carries. Both ids are nullable in the column and
// typed in the entry, and a row that named them must come back naming them —
// otherwise "which passport did this" is unanswerable for exactly the writes
// that most need to be answerable.
func TestAnAgentsRowKeepsThePassportAndThePersonBehindIt(t *testing.T) {
	e := setupAuditBoundary(t)
	id, passport, onBehalfOf := e.agentRow(t, boundaryEarlier)

	for _, entry := range e.entries(t) {
		if entry.ID != id {
			continue
		}
		if entry.PassportID == nil || entry.PassportID.UUID != passport {
			t.Errorf("passport = %v, want %s", entry.PassportID, passport)
		}
		if entry.OnBehalfOf == nil || entry.OnBehalfOf.UUID != onBehalfOf {
			t.Errorf("on_behalf_of = %v, want %s", entry.OnBehalfOf, onBehalfOf)
		}
		if !strings.Contains(string(entry.After), "Architect") {
			t.Errorf("an unscrubbed agent write lost its image: %s", entry.After)
		}
		return
	}
	t.Fatal("the agent's row is absent from the page")
}

// And the same row after a scrub: the envelope still answers who did it, while
// the image it carried does not.
func TestAnAgentsRowKeepsItsEnvelopeAndLosesItsImage(t *testing.T) {
	e := setupAuditBoundary(t)
	id, passport, _ := e.agentRow(t, boundaryEarlier)
	e.auditRow(t, "erase", boundaryErasure, "", "")

	for _, entry := range e.entries(t) {
		if entry.ID != id {
			continue
		}
		if entry.PassportID == nil || entry.PassportID.UUID != passport {
			t.Errorf("the scrub took the passport with the image: %v", entry.PassportID)
		}
		if strings.Contains(string(entry.After), "Architect") {
			t.Errorf("the image survived the scrub: %s", entry.After)
		}
		return
	}
	t.Fatal("the agent's row is absent from the page")
}

// The boundary is a property of the row, not of the query that found it. Each
// filter below narrows to the same pre-scrub edit, and every one of them must
// answer with the image withheld.
func TestTheBoundaryHoldsOnEveryNarrowedPage(t *testing.T) {
	e := setupAuditBoundary(t)
	edit := e.auditRow(t, "update", boundaryEarlier,
		`{"full_name":"Sara Subject"}`, `{"full_name":"Sara Renamed"}`)
	e.auditRow(t, "erase", boundaryErasure, "", "")

	actor := "user:" + e.user.String()
	action, entityType, entityID := "update", "person", e.person.UUID
	window := boundaryEarlier.Add(-time.Hour)
	until := boundaryErasure.Add(time.Hour)

	for name, filter := range map[string]AuditFilter{
		"by actor":  {Actor: &actor},
		"by verb":   {Action: &action},
		"by record": {EntityType: &entityType, EntityID: &entityID},
		"by window": {From: &window, To: &until},
	} {
		t.Run(name, func(t *testing.T) {
			for _, entry := range e.filtered(t, filter) {
				if entry.ID != edit {
					continue
				}
				if strings.Contains(string(entry.Before), "Sara Subject") {
					t.Errorf("%s reached a pre-scrub image: %s", name, entry.Before)
				}
				return
			}
			t.Fatalf("%s returned no row for the edit, so it proved nothing", name)
		})
	}
}

// Paging does not walk around it. The cursor carries a position, not a verdict,
// so a second page is judged by the same rule the first was — and a boundary
// that held only on the page a reader happens to land on would be no boundary.
func TestTheBoundaryHoldsOnThePageAfterTheFirst(t *testing.T) {
	e := setupAuditBoundary(t)
	for i := range 4 {
		e.auditRow(t, "update", boundaryEarlier.Add(time.Duration(i)*time.Minute),
			`{"full_name":"Sara Subject"}`, `{"full_name":"Sara Renamed"}`)
	}
	e.auditRow(t, "erase", boundaryErasure, "", "")

	entityType, entityID, limit := "person", e.person.UUID, 2
	first, err := ListAuditLog(e.ctx, e.db, AuditFilter{
		EntityType: &entityType, EntityID: &entityID, Limit: &limit,
	})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("the fixture did not page: %d entries, more=%t", len(first.Entries), first.HasMore)
	}
	next, err := ListAuditLog(e.ctx, e.db, AuditFilter{
		EntityType: &entityType, EntityID: &entityID, Limit: &limit, Cursor: &first.NextCursor,
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(next.Entries) == 0 {
		t.Fatal("the second page is empty, so it proved nothing")
	}
	for _, entry := range next.Entries {
		if strings.Contains(string(entry.Before), "Sara Subject") {
			t.Errorf("a pre-scrub image reached page two: %s", entry.Before)
		}
	}
}

// The two entity types the boundary could not reach until the erasure tombstoned
// them. An attachment's create image carries the FILENAME somebody typed —
// routinely the subject's own name — and a scheduled send's carries the message
// SUBJECT. Neither type is projected by field history, so the compliance log is
// the only door they were ever readable through, and the boundary only fires
// where a tombstone exists.
func TestTheCollateralTypesAreTombstonedSoTheBoundaryReachesThem(t *testing.T) {
	for _, entityType := range []string{"attachment", "scheduled_send"} {
		t.Run(entityType, func(t *testing.T) {
			e := setupAuditBoundary(t)
			id := ids.NewV7()
			e.collateralImage(t, entityType, id)
			if _, err := e.owner.Exec(context.Background(),
				`INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, occurred_at)
				 VALUES ($1, 'human', 'user:'||$2::text, 'erase', $3, $4, $5)`,
				ids.NewV7(), e.user, entityType, id, boundaryErasure); err != nil {
				t.Fatalf("seeding the %s tombstone: %v", entityType, err)
			}

			limit := 50
			page, err := ListAuditLog(e.ctx, e.db, AuditFilter{EntityType: &entityType, EntityID: &id, Limit: &limit})
			if err != nil {
				t.Fatalf("ListAuditLog: %v", err)
			}
			// The create row must be THERE and withheld, which is what
			// namesTheSubject insists on: absence alone would pass on an empty
			// page, or on one holding only the tombstone.
			if namesTheSubject(t, page) {
				t.Errorf("an erased %s still names the subject", entityType)
			}
		})
	}
}

// AN ATTACHMENT WHOSE ROW IS GONE IS GOVERNED, not ungoverned.
//
// The audit trail outlives what it describes, so the route's question — is this
// content governed by an activity's audience — is asked of a row that may no
// longer be there, and a scalar subquery over no row answers NULL. NULL is not a
// third answer: it is not knowing, and not knowing has to read as governed.
//
// Reading it the other way would make a vanished attachment's image MORE
// readable than a live one's, which is the wrong direction for a boundary. No
// erasure here on purpose — a tombstone withholds the image on its own, so a
// case that had one would pass whichever way this resolved.
func TestAnAttachmentWithNoRowLeftIsWithheldRatherThanReleased(t *testing.T) {
	e := setupAuditBoundary(t)
	entityType := "attachment"
	id := ids.NewV7()
	e.collateralImage(t, entityType, id)

	limit := 50
	page, err := ListAuditLog(e.ctx, e.db, AuditFilter{EntityType: &entityType, EntityID: &id, Limit: &limit})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if namesTheSubject(t, page) {
		t.Error("an attachment whose row is gone still names the subject — a boundary that opens " +
			"when it cannot tell is not a boundary")
	}
}

// collateralImage seeds a create row over one of the collateral types, carrying
// the image both boundary cases are about withholding. A helper rather than a
// block in each test, so a change to the fixture reaches both readings of it.
func (e *auditBoundaryEnv) collateralImage(t *testing.T, entityType string, id ids.UUID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, after, occurred_at)
		 VALUES ($1, 'human', 'user:'||$2::text, 'create', $3, $4,
		         '{"filename":"sara-subject-passport.pdf","subject":"Sara Subject contract"}'::jsonb, $5)`,
		ids.NewV7(), e.user, entityType, id, boundaryEarlier); err != nil {
		t.Fatalf("seeding the %s image: %v", entityType, err)
	}
}

// namesTheSubject reports whether the create row on the page still carries the
// subject's name, failing the test if there is no create row to judge — absence
// would otherwise pass on an empty page, or on one holding only a tombstone.
func namesTheSubject(t *testing.T, page AuditPage) bool {
	t.Helper()
	for _, entry := range page.Entries {
		if entry.Action != "create" {
			continue
		}
		return strings.Contains(string(entry.After), "Sara Subject")
	}
	t.Fatal("the create row is absent from the page, so this proved nothing")
	return false
}
