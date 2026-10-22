// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The two boxes on a LinkedIn match card, "Use as title" and "Use as employer".
// The real import and matcher stage the card. The approvals engine decides it
// with the edit the card sends.

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const linkedInFieldsExport = `First Name,Last Name,URL,Email Address,Company,Position,Connected On
Andreas,Müller,https://www.linkedin.com/in/amueller,,Acme GmbH,Head of Sales,02 Feb 2023
`

// linkedInEmailExport is the same connection with the address the contact has.
var linkedInEmailExport = strings.Replace(linkedInFieldsExport, "amueller,,", "amueller,andreas@acme.test,", 1)

// linkedInFieldsCase is one staged card and the records it is about.
type linkedInFieldsCase struct {
	e        *integration.Env
	ctx      context.Context
	svc      *approvals.Service
	store    *contacts.Store
	contact  ids.UUID
	acme     ids.UUID
	approval ids.ApprovalID
}

// stageLinkedInFieldsCase imports one connection at Acme with a position. It
// stages its folded-name match onto a contact with a non-primary employment at
// Acme, which the name tier requires. title is the contact's title. employer
// names a company that is already their primary employer; empty means none.
func stageLinkedInFieldsCase(t *testing.T, title, employer string) *linkedInFieldsCase {
	t.Helper()
	c := newLinkedInFieldsCase(t, title, employer)
	c.employ(t, c.acme, false)
	c.importAndStage(c.ctx, t, linkedInFieldsExport)
	return c
}

// stageLinkedInEmailMatch stages the address tier's match, which a caller who
// may not edit contacts leaves as a suggestion, and which needs no employment.
func stageLinkedInEmailMatch(t *testing.T, export string) *linkedInFieldsCase {
	t.Helper()
	c := newLinkedInFieldsCase(t, "", "")
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["contact"] = principal.ObjectGrant{Read: true, Create: true}
	c.importAndStage(c.e.As(c.e.Rep1, []ids.UUID{c.e.Team1}, perms), t, export)
	return c
}

func newLinkedInFieldsCase(t *testing.T, title, employer string) *linkedInFieldsCase {
	t.Helper()
	e := integration.Setup(t)
	c := &linkedInFieldsCase{e: e, ctx: e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)}
	c.store = contacts.NewStore(e.DB())
	c.svc = approvalsServiceWithEffects(e.Pool)
	c.acme = c.company(t, "Acme GmbH")
	in := contacts.CreateContactInput{
		FullName: "Andreas Muller", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "andreas@acme.test", EmailType: "work", IsPrimary: true}},
	}
	if title != "" {
		in.Title = &title
	}
	contact, err := e.Contacts.CreateContact(c.ctx, in)
	if err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	c.contact = ids.UUID(contact.Id)
	if employer != "" {
		c.employ(t, c.company(t, employer), true)
	}
	return c
}

func (c *linkedInFieldsCase) importAndStage(ctx context.Context, t *testing.T, export string) {
	t.Helper()
	if _, err := c.store.ImportLinkedInConnections(ctx, strings.NewReader(export)); err != nil {
		t.Fatalf("importing the export: %v", err)
	}
	c.restageAs(ctx, t)
	c.approval = onlyPendingLinkedInMatch(t, c.e)
}

func (c *linkedInFieldsCase) restage(t *testing.T) {
	t.Helper()
	c.restageAs(c.ctx, t)
}

func (c *linkedInFieldsCase) restageAs(ctx context.Context, t *testing.T) {
	t.Helper()
	if _, err := c.store.MatchLinkedInConnections(ctx, c.e.Rep1); err != nil {
		t.Fatalf("matching: %v", err)
	}
	if _, err := StageLinkedInMatches(ctx, c.e.Pool, c.svc, c.store); err != nil {
		t.Fatalf("staging: %v", err)
	}
}

func (c *linkedInFieldsCase) company(t *testing.T, name string) ids.UUID {
	t.Helper()
	company, err := c.e.Contacts.CreateCompany(c.ctx, contacts.CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return ids.UUID(company.Id)
}

// employ records an employment through the ordinary relationship create.
func (c *linkedInFieldsCase) employ(t *testing.T, company ids.UUID, primary bool) {
	t.Helper()
	contactID, companyID := ids.ContactID{UUID: c.contact}, ids.CompanyID{UUID: company}
	if _, err := c.e.Contacts.CreateRelationship(c.ctx, contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, IsCurrentPrimary: &primary, Source: "manual",
	}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}
}

//craft:ignore naked-any a staged change is an open JSON object, and the card edits it as one
func (c *linkedInFieldsCase) payload(t *testing.T) map[string]any {
	t.Helper()
	var raw []byte
	if err := database.WithWorkspaceTx(c.e.Admin(), c.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT proposed_change FROM approval WHERE id = $1`, c.approval).Scan(&raw)
	}); err != nil {
		t.Fatalf("reading the staged change: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unreadable staged change: %v", err)
	}
	return out
}

// approveWith sends the card's edit: the whole staged change, with the members
// given here replaced.
//
//craft:ignore naked-any the edit is the open staged object with some members replaced
func (c *linkedInFieldsCase) approveWith(ctx context.Context, t *testing.T, changes map[string]any) error {
	t.Helper()
	edited := c.payload(t)
	maps.Copy(edited, changes)
	raw, err := json.Marshal(edited)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.svc.DecideEdited(ctx, c.approval, raw)
	return err
}

func (c *linkedInFieldsCase) title(t *testing.T) string {
	t.Helper()
	var title *string
	c.query(t, `SELECT title FROM contact WHERE id = $1`, []any{c.contact}, &title)
	if title == nil {
		return ""
	}
	return *title
}

func (c *linkedInFieldsCase) connectionStatus(t *testing.T) string {
	t.Helper()
	var status string
	c.query(t, `SELECT match_status FROM linkedin_connection`, nil, &status)
	return status
}

// primaryEmployer is the company and role of the contact's current primary
// employment, and its source.
func (c *linkedInFieldsCase) primaryEmployer(t *testing.T) (company ids.UUID, role, source string) {
	t.Helper()
	c.query(t, `SELECT company_id, coalesce(role, ''), source FROM relationship
		 WHERE contact_id = $1 AND `+employment.CurrentPrimarySlotSQL(""),
		[]any{c.contact}, &company, &role, &source)
	return company, role, source
}

func (c *linkedInFieldsCase) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	c.query(t, sql, args, &n)
	return n
}

func (c *linkedInFieldsCase) query(t *testing.T, sql string, args []any, dest ...any) {
	t.Helper()
	if err := database.WithWorkspaceTx(c.e.Admin(), c.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), sql, args...).Scan(dest...)
	}); err != nil {
		t.Fatalf("reading %q: %v", sql, err)
	}
}

// decidedEvent is the linkedin_match.decided payload the approval wrote.
func (c *linkedInFieldsCase) decidedEvent(t *testing.T) (title, employer bool) {
	t.Helper()
	c.query(t, `SELECT (envelope->'payload'->>'title_written')::bool, (envelope->'payload'->>'employer_written')::bool
		  FROM event_outbox WHERE envelope->>'type' = 'linkedin_match.decided'`, nil, &title, &employer)
	return title, employer
}

func TestUseAsTitleIsStagedTickedForAContactWithNoTitleAndWritesThePosition(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	staged := c.payload(t)
	if staged["connection_position"] != "Head of Sales" || staged["use_as_title"] != true {
		t.Fatalf("the card is staged as %v, want the export's position with the title box ticked", staged)
	}
	if _, err := c.svc.Decide(c.ctx, c.approval, true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}
	if got := c.title(t); got != "Head of Sales" {
		t.Errorf("the contact's title is %q, want the export's position", got)
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1
		 AND after ? 'title'`, c.contact); n != 1 {
		t.Errorf("%d contact audit rows record the title, want exactly 1", n)
	}
	var source, capturedBy, evidence string
	c.query(t, `SELECT source, captured_by, evidence_ref FROM field_provenance
		 WHERE object_type = 'contact' AND object_id = $1 AND field_name = 'title'`,
		[]any{c.contact}, &source, &capturedBy, &evidence)
	if source != "linkedin_export" || capturedBy != "human:"+c.e.Rep1.String() || evidence != onlyLinkedInConnection(t, c.e).String() {
		t.Errorf("the title's provenance is %s by %s from %s, want linkedin_export by the decider from the connection",
			source, capturedBy, evidence)
	}
	// The contact had no current employer either, so both boxes were ticked.
	if title, employer := c.decidedEvent(t); !title || !employer {
		t.Errorf("the decision event says title_written=%v employer_written=%v, want both true", title, employer)
	}
}

func TestAContactWithATitleKeepsItUnlessTheBoxIsTicked(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "Founder", "")
	if staged := c.payload(t); staged["use_as_title"] != false || staged["contact_title"] != "Founder" {
		t.Fatalf("the card is staged as %v, want the contact's title beside an unticked box", staged)
	}
	if _, err := c.svc.Decide(c.ctx, c.approval, true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}
	if got := c.title(t); got != "Founder" {
		t.Errorf("an unticked box changed the title to %q", got)
	}
	if title, _ := c.decidedEvent(t); title {
		t.Error("the decision event says the title was written by an unticked box")
	}
}

func TestTickingUseAsTitleReplacesATitleAndKeepsTheOldOneInTheAudit(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "Founder", "")
	if err := c.approveWith(c.ctx, t, map[string]any{"use_as_title": true}); err != nil {
		t.Fatalf("approving with the box ticked: %v", err)
	}
	if got := c.title(t); got != "Head of Sales" {
		t.Errorf("the contact's title is %q, want the export's position", got)
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1
		 AND before->>'title' = 'Founder' AND after->>'title' = 'Head of Sales'`, c.contact); n != 1 {
		t.Errorf("%d audit rows hold Founder → Head of Sales, want exactly 1", n)
	}
}

func TestUseAsEmployerMakesThePlacedCompanyTheCurrentPrimaryEmployer(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	staged := c.payload(t)
	if staged["employer_company_id"] != c.acme.String() || staged["use_as_employer"] != true {
		t.Fatalf("the card is staged as %v, want Acme offered with the employer box ticked", staged)
	}
	if _, err := c.svc.Decide(c.ctx, c.approval, true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}
	company, role, _ := c.primaryEmployer(t)
	if company != c.acme || role != "Head of Sales" {
		t.Errorf("the current primary employment is %s as %q, want the Acme employment promoted with the position as its role",
			company, role)
	}
	if n := c.count(t, `SELECT count(*) FROM field_provenance fp JOIN relationship r ON r.id = fp.object_id
		 WHERE fp.object_type = 'relationship' AND fp.field_name IN ('role', 'is_current_primary')
		   AND fp.source = 'linkedin_export' AND fp.captured_by = $2 AND r.contact_id = $1`,
		c.contact, "human:"+c.e.Rep1.String()); n != 2 {
		t.Errorf("%d provenance rows name the export for the role and the primary flag, want 2", n)
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log a JOIN relationship r ON r.id = a.entity_id
		 WHERE a.entity_type = 'relationship' AND a.action = 'update' AND r.contact_id = $1`, c.contact); n != 1 {
		t.Errorf("%d update audit rows for the promoted employment, want 1", n)
	}
	if _, employer := c.decidedEvent(t); !employer {
		t.Error("the decision event says no employer was written")
	}
}

func TestTickingUseAsEmployerReplacesTheCurrentPrimaryEmployer(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "Old Employer GmbH")
	if staged := c.payload(t); staged["use_as_employer"] != false || staged["employer_relationship_id"] == nil {
		t.Fatalf("the card is staged as %v, want the current employment named beside an unticked box", staged)
	}
	if err := c.approveWith(c.ctx, t, map[string]any{"use_as_employer": true}); err != nil {
		t.Fatalf("approving with the box ticked: %v", err)
	}
	if company, _, _ := c.primaryEmployer(t); company != c.acme {
		t.Errorf("the current primary employer is %s, want Acme", company)
	}
}

func TestAConnectionPlacedAtNoCompanyOffersNoEmployer(t *testing.T) {
	c := stageLinkedInEmailMatch(t, strings.Replace(linkedInEmailExport, "Acme GmbH", "Unknown Ltd", 1))
	if staged := c.payload(t); staged["employer_company_id"] != nil || staged["use_as_employer"] != false {
		t.Errorf("the card is staged as %v, want no employer offered", staged)
	}
}

func TestUseAsEmployerCreatesTheEmploymentWhenTheContactHasNoneThere(t *testing.T) {
	c := stageLinkedInEmailMatch(t, linkedInEmailExport)
	if staged := c.payload(t); staged["employer_company_id"] != c.acme.String() || staged["use_as_employer"] != true {
		t.Fatalf("the card is staged as %v, want Acme offered with the employer box ticked", staged)
	}
	if _, err := c.svc.Decide(c.ctx, c.approval, true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}
	company, role, source := c.primaryEmployer(t)
	if company != c.acme || role != "Head of Sales" || source != "linkedin_export" {
		t.Errorf("the current primary employment is %s as %q from %s, want a new Acme edge as Head of Sales from linkedin_export",
			company, role, source)
	}
}

func TestAnEditToAnythingButTheBoxesIsRefusedAndWritesNothing(t *testing.T) {
	for _, edit := range []map[string]any{
		{"connection_position": "Chief Executive"},
		{"contact_title": "Founder"},
		{"employer_company_id": ids.NewV7().String()},
	} {
		c := stageLinkedInFieldsCase(t, "", "")
		err := c.approveWith(c.ctx, t, edit)
		var invalid *approvals.InvalidEditError
		var retargeted *approvals.RetargetedEditError
		if !errors.As(err, &invalid) && !errors.As(err, &retargeted) {
			t.Errorf("an edit of %v answered %v, want it refused", edit, err)
		}
		if got := c.title(t); got != "" || c.connectionStatus(t) != "suggested" {
			t.Errorf("a refused edit of %v left title %q and connection %s", edit, got, c.connectionStatus(t))
		}
	}
}

func TestATitleSetWhileTheCardWaitedIsNeverReplaced(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	if _, err := c.e.Contacts.UpdateContact(c.ctx, ids.ContactID{UUID: c.contact},
		contacts.UpdateContactInput{Title: new("VP Sales")}); err != nil {
		t.Fatalf("a human sets the title: %v", err)
	}
	if _, err := c.svc.Decide(c.ctx, c.approval, true, nil); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("approving answered %v, want a conflict", err)
	}
	if got := c.title(t); got != "VP Sales" {
		t.Errorf("the human's title became %q", got)
	}
	if status := c.connectionStatus(t); status != "suggested" {
		t.Errorf("the refused approval left the connection %s, want it still suggested", status)
	}
}

func TestAnEmployerSetWhileTheCardWaitedIsNeverReplaced(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	newer := c.company(t, "Newer GmbH")
	c.employ(t, newer, true)
	if _, err := c.svc.Decide(c.ctx, c.approval, true, nil); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("approving answered %v, want a conflict", err)
	}
	if company, _, _ := c.primaryEmployer(t); company != newer {
		t.Errorf("the human's employer was replaced by %s", company)
	}
	if got := c.title(t); got != "" {
		t.Errorf("the refused approval still wrote the title %q", got)
	}
}

// relationshipGrant is a decider with every grant but this one on employment
// edges.
func (c *linkedInFieldsCase) relationshipGrant(grant principal.ObjectGrant) context.Context {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["relationship"] = grant
	return c.e.As(c.e.Rep1, []ids.UUID{c.e.Team1}, perms)
}

func TestADeciderWhoMayNotCreateTheEmploymentIsRefusedBeforeAnyWrite(t *testing.T) {
	c := stageLinkedInEmailMatch(t, linkedInEmailExport)
	decider := c.relationshipGrant(principal.ObjectGrant{Read: true, Update: true})
	if _, err := c.svc.Decide(decider, c.approval, true, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("approving a new employment without relationship:create answered %v, want a permission refusal", err)
	}
	if got := c.title(t); got != "" || c.connectionStatus(t) != "suggested" {
		t.Errorf("the refused approval left title %q and connection %s", got, c.connectionStatus(t))
	}
	// The plain match stays theirs to confirm.
	if err := c.approveWith(decider, t, map[string]any{"use_as_employer": false}); err != nil {
		t.Fatalf("approving without the employer box: %v", err)
	}
	if got := c.title(t); got != "Head of Sales" {
		t.Errorf("the title is %q, want the export's position", got)
	}
}

func TestADeciderWhoMayCreateButNotUpdateEmploymentsIsRefusedThePromote(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	decider := c.relationshipGrant(principal.ObjectGrant{Read: true, Create: true})
	if _, err := c.svc.Decide(decider, c.approval, true, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("promoting the Acme employment without relationship:update answered %v, want a permission refusal", err)
	}
	if n := c.count(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND `+
		employment.CurrentPrimarySlotSQL(""), c.contact); n != 0 {
		t.Errorf("the refused promote left %d current primary employments, want none", n)
	}
	if got := c.title(t); got != "" || c.connectionStatus(t) != "suggested" {
		t.Errorf("the refused approval left title %q and connection %s", got, c.connectionStatus(t))
	}
}

func TestADeciderWhoMayUpdateButNotCreateEmploymentsPromotesOne(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	decider := c.relationshipGrant(principal.ObjectGrant{Read: true, Update: true})
	if _, err := c.svc.Decide(decider, c.approval, true, nil); err != nil {
		t.Fatalf("promoting with relationship:update alone: %v", err)
	}
	if company, _, _ := c.primaryEmployer(t); company != c.acme {
		t.Errorf("the current primary employer is %s, want the Acme employment promoted", company)
	}
}

func TestADeciderWhoMayCreateButNotUpdateEmploymentsCreatesOne(t *testing.T) {
	c := stageLinkedInEmailMatch(t, linkedInEmailExport)
	decider := c.relationshipGrant(principal.ObjectGrant{Read: true, Create: true})
	if _, err := c.svc.Decide(decider, c.approval, true, nil); err != nil {
		t.Fatalf("creating the employment with relationship:create alone: %v", err)
	}
	if company, _, source := c.primaryEmployer(t); company != c.acme || source != "linkedin_export" {
		t.Errorf("the current primary employment is %s from %s, want a new Acme edge from linkedin_export", company, source)
	}
}

func TestAPromotedEmploymentKeepsItsAuthor(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	colleague := c.e.As(c.e.AdminUser, nil, integration.AdminPerms)
	if _, err := c.svc.Decide(colleague, c.approval, true, nil); err != nil {
		t.Fatalf("a colleague approving: %v", err)
	}
	var author string
	c.query(t, `SELECT captured_by FROM relationship WHERE contact_id = $1 AND company_id = $2`,
		[]any{c.contact, c.acme}, &author)
	if want := "human:" + c.e.Rep1.String(); author != want {
		t.Errorf("the promoted employment belongs to %s, want its author %s kept", author, want)
	}
}

func TestADeciderWhoCannotSeeTheCompanyIsRefusedTheEmployer(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	if err := database.WithWorkspaceTx(c.e.Admin(), c.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE company SET visibility = 'owner', owner_id = $2 WHERE id = $1`, c.acme, c.e.Rep1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	colleague := c.e.As(c.e.AdminUser, nil, integration.AdminPerms)
	if _, err := c.svc.Decide(colleague, c.approval, true, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a colleague who cannot see Acme approving the employer box answered %v, want not found", err)
	}
	if got := c.title(t); got != "" || c.connectionStatus(t) != "suggested" {
		t.Errorf("the refused approval left title %q and connection %s", got, c.connectionStatus(t))
	}
	// Refused while still pending, so the colleague can untick the box.
	if err := c.approveWith(colleague, t, map[string]any{"use_as_employer": false}); err != nil {
		t.Fatalf("approving without the employer box: %v", err)
	}
}

func TestAFailedEmploymentTakesTheTitleWithIt(t *testing.T) {
	c := stageLinkedInEmailMatch(t, linkedInEmailExport)
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(c.ctx, `ALTER TABLE relationship ADD CONSTRAINT refuse_export_employment_test
		CHECK (source <> 'linkedin_export') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `ALTER TABLE relationship DROP CONSTRAINT refuse_export_employment_test`); err != nil {
			t.Error(err)
		}
	})
	if _, err := c.svc.Decide(c.ctx, c.approval, true, nil); err == nil {
		t.Fatal("approving reported success while the employment could not be written")
	}
	if got := c.title(t); got != "" || c.connectionStatus(t) != "suggested" {
		t.Errorf("the failed approval left title %q and connection %s", got, c.connectionStatus(t))
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1 AND after ? 'title'`,
		c.contact); n != 0 {
		t.Errorf("%d title audit rows survived the failed approval", n)
	}
}

func TestALaterExportNeverWritesTheContactAgain(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	if _, err := c.svc.Decide(c.ctx, c.approval, true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}
	// Same identity with a new position, then a new company: neither writes.
	later := strings.Replace(linkedInFieldsExport, "Head of Sales", "Chief Revenue Officer", 1)
	for _, export := range []string{later, strings.Replace(later, "Acme GmbH", "Other AG", 1)} {
		if _, err := c.store.ImportLinkedInConnections(c.ctx, strings.NewReader(export)); err != nil {
			t.Fatalf("re-importing: %v", err)
		}
		c.restage(t)
	}
	if got := c.title(t); got != "Head of Sales" {
		t.Errorf("a later export changed the title to %q", got)
	}
	// The matcher proposes no second connection for a contact this member has
	// already confirmed one onto, so nothing is left to tick.
	if n := pendingLinkedInMatchCount(t, c.e); n != 0 {
		t.Errorf("%d cards wait after the later exports, want none for a confirmed contact", n)
	}
}

func TestACardStagedBeforeTheBoxesAppliesOnlyTheLink(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	if err := database.WithWorkspaceTx(c.e.Admin(), c.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE approval SET proposed_change = proposed_change
			- 'use_as_title' - 'use_as_employer' - 'connection_position' - 'contact_title'
			- 'employer_company_id' - 'employer_relationship_id' WHERE id = $1`, c.approval)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var invalid *approvals.InvalidEditError
	if err := c.approveWith(c.ctx, t, map[string]any{"use_as_title": true}); !errors.As(err, &invalid) {
		t.Fatalf("adding a box to an old card answered %v, want an invalid edit", err)
	}
	if _, err := c.svc.Decide(c.ctx, c.approval, true, nil); err != nil {
		t.Fatalf("approving the old card: %v", err)
	}
	if got := c.title(t); got != "" || c.connectionStatus(t) != "confirmed" {
		t.Errorf("the old card left title %q and connection %s, want only the link", got, c.connectionStatus(t))
	}
}

func TestAPositionTheExportChangedSinceStagingIsNotWritten(t *testing.T) {
	c := stageLinkedInFieldsCase(t, "", "")
	later := strings.Replace(linkedInFieldsExport, "Head of Sales", "Chief Revenue Officer", 1)
	if _, err := c.store.ImportLinkedInConnections(c.ctx, strings.NewReader(later)); err != nil {
		t.Fatalf("re-importing: %v", err)
	}
	if _, err := c.svc.Decide(c.ctx, c.approval, true, nil); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("approving the card staged with the old position answered %v, want a conflict", err)
	}
	if got := c.title(t); got != "" || c.connectionStatus(t) != "suggested" {
		t.Errorf("the refused approval left title %q and connection %s", got, c.connectionStatus(t))
	}
}
