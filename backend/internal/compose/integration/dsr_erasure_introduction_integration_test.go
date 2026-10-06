// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// introErasureAdmin is the seat this suite acts as: enough to ask for an introduction
// about a contact and then erase them. Spelled here rather than widened on the shared
// fixture, so a grant added for this suite cannot quietly widen every other one.
var introErasureAdmin = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"contact":      {Create: true, Read: true, Update: true, Delete: true},
		"introduction": {Create: true, Read: true, Update: true, Delete: true},
	},
	RowScope: principal.RowScopeAll,
}

// What colleagues wrote about the subject while asking each other for an introduction
// goes when the subject does.
//
// intro_request.contact_id references contact ON DELETE CASCADE, which never fires in
// an erasure: the contact is anonymized rather than deleted, so nothing in the cascade
// reached the row and four free-text columns went on describing a contact whose name
// had been wiped.
func TestErasingASubjectClearsWhatAnIntroductionSaidAboutThem(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, introErasureAdmin)
	subject := e.SeedContact(t, "Rita Reviewer", &e.Rep1)

	note := "Rita runs platform at Acme and owns the budget"
	intros := introductions.NewStore(e.DB(), time.Now)
	requestID, err := intros.Create(ctx, introductions.NewRequest{
		ContactID:       subject,
		IntroducerUser:  e.Rep2,
		RouteType:       "direct",
		InternalReason:  "Rita turned us down last year; a warm path is worth more",
		ValueForTarget:  "Rita gets a reference architecture she asked about",
		ForwardableNote: note,
		NoteGeneratedBy: "human",
		FallbackPolicy:  "none",
		DueAt:           time.Now().Add(72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("seeding the introduction request: %v", err)
	}
	// Decided, so decision_reason is non-NULL: the eraser keeps a tombstone where a
	// reason was given and NULL where none was, and only a decided request exercises
	// the first branch.
	if err := intros.Decide(e.As(e.Rep2, nil, introErasureAdmin), requestID,
		introductions.StatusDeclined, "Rita already said no to Acme last year", nil, 1); err != nil {
		t.Fatalf("deciding the introduction: %v", err)
	}

	if n := introRowsNaming(t, e, "Rita"); n != 1 {
		t.Fatalf("the introduction's prose does not name the subject (%d rows), so this test "+
			"proves nothing", n)
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(ctx, subject, "subject request"); err != nil {
		t.Fatalf("EraseContact → %v", err)
	}

	if n := introRowsNaming(t, e, "Rita"); n != 0 {
		t.Errorf("%d introduction row(s) still describe the erased subject by name — the erasure "+
			"reported their data destroyed while a colleague's ask reads out who they were", n)
	}
	// The decision's own prose took the tombstone rather than a blank or a NULL: a
	// reason WAS given, and the record of that is the introducer's, not the subject's.
	var decided string
	if err := e.DB().Pool().QueryRow(t.Context(),
		`SELECT coalesce(decision_reason, '') FROM intro_request WHERE id = $1`,
		requestID).Scan(&decided); err != nil {
		t.Fatalf("reading back the decision: %v", err)
	}
	if decided == "" || strings.Contains(decided, "Rita") {
		t.Errorf("the decision reason reads %q — a reason that was given should survive as a "+
			"tombstone, naming nobody", decided)
	}
	// The row itself stays: that an introduction was asked for and what came of it is
	// the colleagues' own record of their work.
	if n := e.WsCount(t, `SELECT count(*) FROM intro_request WHERE contact_id = $1`, subject); n != 1 {
		t.Errorf("the request row was destroyed (%d left), taking a record of the colleagues' own "+
			"work with data that was the subject's", n)
	}
}

// introRowsNaming counts the introduction rows whose free prose still contains a
// string, read past every API gate: the question is what the database holds.
func introRowsNaming(t *testing.T, e *Env, words string) int {
	t.Helper()
	return e.WsCount(t, `
		SELECT count(*) FROM intro_request
		 WHERE forwardable_note || ' ' || internal_reason || ' ' || value_for_target
		       || ' ' || coalesce(decision_reason, '') ILIKE '%' || $1 || '%'`, words)
}
