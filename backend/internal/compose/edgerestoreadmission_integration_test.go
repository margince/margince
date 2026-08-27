// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the WRITE path admits as an entry of one record's history.
//
// The reversal route addresses a row the path record does not own: a link's
// audit row sits on ('relationship', edge_id) and appears on the history of both
// records it joins. So the write holds two identities apart — the record whose
// history is open, and the target row — and privacy.HistoryServesEntry is the one
// gate deciding whether the second belongs to the first's history.
//
// It is the disclosure-critical half. Bound by id alone, a caller holding an
// audit id could probe for a link whose other end they may not see, or reach an
// entry from a record's history that is not this record's at all. Every case here
// is one of those probes, and every one of them must answer ABSENT: a refusal
// names a reason, and naming a reason is proof the entry exists. The last case is
// the control, so the three above it cannot be passing because everything is
// refused.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedEmploymentEdge links a person to an organization through the people
// store's own write path — the one that stamps the audit row under test. A
// hand-rolled INSERT would prove nothing about production: the action, the
// entity_type and the image on that row are exactly what the admission reads.
//
// seededEdgeRole is the role every edge here is created with. A constant rather
// than a parameter: no case varies it, and a parameter would suggest the
// admission cares what the role says when it does not.
const seededEdgeRole = "cto"

func seedEmploymentEdge(t *testing.T, e *integration.Env, person, org ids.UUID) ids.UUID {
	t.Helper()
	role := seededEdgeRole
	personID, orgID := ids.From[ids.PersonKind](person), ids.From[ids.OrganizationKind](org)
	edge, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "employment", PersonID: &personID, OrganizationID: &orgID,
		Role: &role, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed the link through the real writer: %v", err)
	}
	return edge.ID
}

// edgeIsLive reports whether the link is still un-archived, which is how "the
// reverse committed" is observable without reading the reverse's own answer.
func edgeIsLive(t *testing.T, e *integration.Env, edge ids.UUID) bool {
	t.Helper()
	admin := e.Admin()
	var live bool
	if err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(admin,
			`SELECT archived_at IS NULL FROM relationship WHERE id = $1`, edge).Scan(&live)
	}); err != nil {
		t.Fatalf("read whether link %s is live: %v", edge, err)
	}
	return live
}

// answeredAbsent asserts the reversal answered "no such entry" and nothing else.
//
// Three outcomes are separately wrong and separately checked, because they leak
// different things. A REFUSAL names why the entry cannot be put back, which says
// the entry is there. A 403 says the caller lacks authority over something,
// which says the same. And any other error would be a fault dressed as a
// decision.
func answeredAbsent(t *testing.T, err error, probe string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s was reversed", probe)
	}
	var refusal RefusedRestore
	if errors.As(err, &refusal) {
		t.Fatalf("%s answered the refusal %q; naming a reason is proof the entry exists, "+
			"and this caller may not learn that", probe, refusal.Reason)
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("%s answered a permission denial (403); a refusal is proof the entry "+
			"exists, so the admission must answer absence", probe)
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("%s answered %v, want ErrNotFound", probe, err)
	}
}

// The other end is a company this caller cannot read, so the link's entry is not
// an entry of the person's history for them — and the reverse cannot say so.
func TestALinkWhoseOtherEndTheCallerCannotSeeIsNotReversible(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Secret Holdings GmbH", nil)
	edge := seedEmploymentEdge(t, e, person, org)
	// Captured privately by Rep1. Capture privacy does not yield to
	// row_scope=all, so even the admin reading the person cannot see the company.
	e.MakeCapturePrivate(t, "organization", org, e.Rep1)

	auditID := latestAuditRowID(t, e, edgeEntityType, edge, "create")
	_, err := restoreSeamFor(e).Restore(e.Admin(), "person", person, auditID,
		currentVersion(t, e, "person", person))
	answeredAbsent(t, err, "reversing a link whose company the caller cannot read")
	if !edgeIsLive(t, e, edge) {
		t.Error("the refused reverse removed the link anyway")
	}
}

// The other end was ERASED, and its employment image holds that subject's role
// and dates. Reversing it from the company's page would act on data an Art. 17
// certificate said was gone, so the entry is not served there at all.
func TestALinkWhoseOtherEndWasErasedIsNotReversible(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Selma Subject", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	edge := seedEmploymentEdge(t, e, person, org)
	auditID := latestAuditRowID(t, e, edgeEntityType, edge, "create")

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(),
		person, "art-17"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	_, err := restoreSeamFor(e).Restore(e.Admin(), "organization", org, auditID,
		currentVersion(t, e, "organization", org))
	// The erasure also archives the subject and its links, so a path that admitted
	// the entry would answer one of the edge branch's own refusals — which names
	// the entry, and is the outcome absence is separated from here.
	answeredAbsent(t, err, "reversing an erased subject's employment from the company")
}

// An audit id from another record's history entirely. The route's path names the
// record whose history is open, and a target row admitted by its id alone would
// let the executor write a record the caller never addressed.
func TestAnEntryFromAnotherRecordsHistoryIsNotReversible(t *testing.T) {
	e := integration.Setup(t)
	subject := e.SeedPerson(t, "Ada Addressed", nil)
	stranger := e.SeedPerson(t, "Otto Elsewhere", nil)
	title := "COO"
	if _, err := e.People.UpdatePerson(e.Admin(), ids.From[ids.PersonKind](stranger),
		people.UpdatePersonInput{Title: &title, Source: "manual"}); err != nil {
		t.Fatalf("change the other record through the real writer: %v", err)
	}
	elsewhere := latestAuditRowID(t, e, "person", stranger, "update")

	_, err := restoreSeamFor(e).Restore(e.Admin(), "person", subject, elsewhere,
		currentVersion(t, e, "person", subject))
	answeredAbsent(t, err, "reversing another record's entry from this record's history")
	if held := titleOf(t, e, stranger); held != title {
		t.Errorf("the other record's title is now %q; the reverse wrote a record the "+
			"caller never addressed", held)
	}
}

// The control. A link the caller CAN see reverses, so the three refusals above
// are the admission answering and not everything being refused.
func TestALinkTheCallerCanSeeIsReversible(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	edge := seedEmploymentEdge(t, e, person, org)

	auditID := latestAuditRowID(t, e, edgeEntityType, edge, "create")
	entry, err := restoreSeamFor(e).Restore(e.Admin(), "person", person, auditID,
		currentVersion(t, e, "person", person))
	if err != nil {
		t.Fatalf("reversing a link the caller can see: %v", err)
	}
	if entry.UndidAuditLogID == nil || *entry.UndidAuditLogID != auditID {
		t.Errorf("the reversal's own line came back as %+v, want the row reversing %s",
			entry, auditID)
	}
	if edgeIsLive(t, e, edge) {
		t.Error("the reverse answered success and left the link live")
	}
}

// titleOf reads one person's title, which is what a cross-record write would
// have moved.
func titleOf(t *testing.T, e *integration.Env, person ids.UUID) string {
	t.Helper()
	admin := e.Admin()
	var title *string
	if err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(admin, `SELECT title FROM person WHERE id = $1`, person).Scan(&title)
	}); err != nil {
		t.Fatalf("read person %s's title: %v", person, err)
	}
	if title == nil {
		return ""
	}
	return *title
}
