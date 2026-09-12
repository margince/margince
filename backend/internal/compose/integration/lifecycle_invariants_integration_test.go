// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Regression lane for the store's cross-cutting invariants: the deal
// lifecycle (amount/currency pairing, terminal-field clearing on reopen,
// owner-change events), activity link-scoping, and the scope-safe dedupe
// 409 — each pins a bug class, not a happy path.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestUpdateDealRejectsAStrandedAmount(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	d, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "No money yet", PipelineID: pipeline, StageID: open, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	amount := int64(5000)
	_, err = e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.UpdateDealInput{AmountMinor: &amount})
	var pairErr *deals.AmountCurrencyPairError
	if !errors.As(err, &pairErr) {
		t.Fatalf("amount without currency → %v, want deals.AmountCurrencyPairError", err)
	}

	// The paired update is accepted, and clearing neither alone either.
	currency := "EUR"
	if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.UpdateDealInput{AmountMinor: &amount, Currency: &currency}); err != nil {
		t.Fatalf("paired amount+currency: %v", err)
	}
}

func TestReopeningAWonDealClearsTerminalFields(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	admin := e.Admin()

	amount, currency := int64(100000), "EUR"
	d, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Round trip", PipelineID: pipeline, StageID: open, Source: "manual",
		AmountMinor: &amount, Currency: &currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.AdvanceDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: WonByImport()}); err != nil {
		t.Fatalf("closing as won: %v", err)
	}
	if _, err := e.Deals.AdvanceDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.AdvanceDealInput{ToStageID: open}); err != nil {
		t.Fatalf("reopening: %v", err)
	}

	owner := OwnerConn(t)
	var status string
	var closedAt, lostReason, fxRate, fxDate *string
	err = owner.QueryRow(context.Background(),
		`SELECT status, closed_at::text, lost_reason, fx_rate_to_base::text, fx_rate_date::text FROM deal WHERE id = $1`,
		ids.UUID(d.Id)).Scan(&status, &closedAt, &lostReason, &fxRate, &fxDate)
	if err != nil {
		t.Fatal(err)
	}
	if status != "open" {
		t.Fatalf("status = %s after reopen, want open", status)
	}
	for name, v := range map[string]*string{"closed_at": closedAt, "lost_reason": lostReason, "fx_rate_to_base": fxRate, "fx_rate_date": fxDate} {
		if v != nil {
			t.Errorf("reopened deal still carries %s = %q — corrupts won/lost reporting", name, *v)
		}
	}
}

func TestOwnerReassignmentEmitsOwnerChanged(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	d, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Handover", PipelineID: pipeline, StageID: open, Source: "manual", OwnerID: userIDPtr(&e.Rep1),
	})
	if err != nil {
		t.Fatal(err)
	}
	name := "Handover (renamed)"
	if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.UpdateDealInput{OwnerID: userIDPtr(&e.Rep2), Name: &name}); err != nil {
		t.Fatal(err)
	}

	owner := OwnerConn(t)
	rows, err := owner.Query(context.Background(),
		`SELECT envelope->>'type', envelope->'payload' FROM event_outbox ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]json.RawMessage{}
	var order []string
	for rows.Next() {
		var typ string
		var payload json.RawMessage
		if err := rows.Scan(&typ, &payload); err != nil {
			t.Fatal(err)
		}
		types[typ] = payload
		order = append(order, typ)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	ownerPayload, ok := types["deal.owner_changed"]
	if !ok {
		t.Fatalf("no deal.owner_changed staged; events seen: %v", order)
	}
	var oc struct {
		From *ids.UUID `json:"from_owner_id"`
		To   ids.UUID  `json:"to_owner_id"`
	}
	if err := json.Unmarshal(ownerPayload, &oc); err != nil {
		t.Fatal(err)
	}
	if oc.From == nil || *oc.From != e.Rep1 || oc.To != e.Rep2 {
		t.Errorf("owner_changed payload %s, want rep1→rep2", ownerPayload)
	}
	// The co-occurring rename still emits deal.updated — WITHOUT the owner
	// field (owner transitions are never folded into the generic event).
	updated, ok := types["deal.updated"]
	if !ok {
		t.Fatal("the co-occurring field change lost its deal.updated")
	}
	var rest map[string]any
	if err := json.Unmarshal(updated, &rest); err != nil {
		t.Fatal(err)
	}
	if _, leaked := rest["owner_id"]; leaked {
		t.Error("deal.updated payload carries owner_id; the transition belongs to deal.owner_changed alone")
	}
}

// An activity's scope is the link walk. A contact is readable by every seat
// unless its capture is private, so the activity Rep1 must not reach is one
// linked only to a capture-private contact of Rep3's.
func TestActivityReadsAreScopedThroughLinks(t *testing.T) {
	e := Setup(t)
	foreignContact := e.SeedContact(t, "Foreign owner", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", foreignContact, e.Rep3)
	myContact := e.SeedContact(t, "Mine", &e.Rep1)
	admin := e.Admin()
	// Only the private contact's owner can link to it.
	foreignOwner := e.As(e.Rep3, []ids.UUID{e.Team2}, AdminPerms)

	secret, _, err := e.Activities.LogActivity(foreignOwner, activities.LogActivityInput{
		Kind: "note", Subject: strPtr("Confidential pricing call"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: foreignContact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	visible, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
		Kind: "note", Subject: strPtr("Team call"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: myContact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	unlinked, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
		Kind: "note", Subject: strPtr("Workspace-wide note"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, repPermsWithActivity())

	// Get: the activity attached to the private contact answers 404.
	if _, err := e.Activities.GetActivity(rep, ids.From[ids.ActivityKind](ids.UUID(secret.Id)), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("foreign-linked activity → %v, want ErrNotFound", err)
	}
	if _, err := e.Activities.GetActivity(rep, ids.From[ids.ActivityKind](ids.UUID(visible.Id)), storekit.LiveOnly); err != nil {
		t.Errorf("team-linked activity → %v, want success", err)
	}

	// List: the timeline never surfaces it, including via the entity filter.
	list, _, err := e.Activities.ListActivities(rep, activities.ListActivitiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[ids.UUID]bool{}
	for _, a := range list {
		seen[ids.UUID(a.Id)] = true
	}
	if seen[ids.UUID(secret.Id)] {
		t.Error("timeline surfaced an activity linked only to a contact the caller cannot read")
	}
	if !seen[ids.UUID(visible.Id)] || !seen[ids.UUID(unlinked.Id)] {
		t.Error("timeline lost a visible or workspace-shared activity")
	}

	// Filtering BY a record is a read OF it, so a record the caller cannot
	// read owes the existence-hiding not-found rather than an empty page.
	// An empty page hides the rows but answers "that record has nothing on it",
	// which is a different sentence and one the caller was not entitled to.
	entityType, entityID := "contact", foreignContact
	_, _, err = e.Activities.ListActivities(rep, activities.ListActivitiesInput{EntityType: &entityType, EntityID: &entityID})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("entity-filter probe on a private contact = %v, want ErrNotFound", err)
	}
}

// A duplicate-email conflict hands back the existing id so a client can open
// the record — unless the caller may not read it. The contact they may not
// read is a capture-private one of another rep's.
func TestDuplicate409DoesNotDiscloseOutOfScopeIDs(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	hidden, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{
		FullName: "Owned elsewhere", OwnerID: userIDPtr(&e.Rep3), Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "taken@example.com", EmailType: "work", IsPrimary: true, Position: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.MakeCapturePrivate(t, "contact", ids.UUID(hidden.Id), e.Rep3)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	_, err = e.Contacts.CreateContact(rep, contacts.CreateContactInput{
		FullName: "Duplicate attempt", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "taken@example.com", EmailType: "work", IsPrimary: true, Position: 1}},
	})
	var dup *contacts.DuplicateEmailError
	if !errors.As(err, &dup) {
		t.Fatalf("duplicate create → %v, want contacts.DuplicateEmailError", err)
	}
	if !dup.ExistingID.IsZero() {
		t.Errorf("409 disclosed out-of-scope id %s", dup.ExistingID)
	}

	// The same conflict against a row the rep CAN see keeps the id — the
	// dedupe UX ("open the existing record") survives for legit cases.
	if _, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{
		FullName: "Teammate's", OwnerID: userIDPtr(&e.Rep2), Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "team@example.com", EmailType: "work", IsPrimary: true, Position: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err = e.Contacts.CreateContact(rep, contacts.CreateContactInput{
		FullName: "Duplicate attempt 2", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "team@example.com", EmailType: "work", IsPrimary: true, Position: 1}},
	})
	if !errors.As(err, &dup) {
		t.Fatalf("visible duplicate → %v, want contacts.DuplicateEmailError", err)
	}
	if dup.ExistingID.IsZero() {
		t.Error("409 for a visible duplicate should carry the existing id")
	}
}

// domainCreateRepPerms is a rep who may CREATE a company and is bounded
// to their team. Both halves are load-bearing: without the create grant the
// probe below never runs and the test reports a permission denial instead of a
// disclosure verdict, and an unbounded caller can see every company, which makes
// the withheld case unreachable.
var domainCreateRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"company":               {Create: true, Read: true, Update: true},
		"contact":               {Create: true, Read: true, Update: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

// A DOMAIN collision answers under the same disclosure rule as an email one,
// and the domain half was untested: every existing case asserted the id IS
// carried, so removing the visibility gate broke nothing.
//
// One test for four doors, because they share `claimedDomainOwner`: creating a
// company, editing its domains, and saving its profile website all reach the
// same probe, so the rule is held in one place for all of them.
func TestDuplicateDomain409DoesNotDiscloseACompanyOutOfScope(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	hidden, err := e.Contacts.CreateCompany(admin, contacts.CreateCompanyInput{
		DisplayName: "Owned elsewhere GmbH", Source: "manual",
		Domains: []contacts.CompanyDomainInput{{Domain: "hidden-owner.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.MakeCapturePrivate(t, "company", ids.UUID(hidden.Id), e.Rep3)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, domainCreateRepPerms)
	_, err = e.Contacts.CreateCompany(rep, contacts.CreateCompanyInput{
		DisplayName: "Duplicate attempt GmbH", Source: "manual",
		Domains: []contacts.CompanyDomainInput{{Domain: "hidden-owner.test", IsPrimary: true}},
	})
	var dup *contacts.DuplicateDomainError
	if !errors.As(err, &dup) {
		t.Fatalf("duplicate domain → %v, want contacts.DuplicateDomainError", err)
	}
	if !dup.ExistingID.IsZero() {
		t.Errorf("409 disclosed an out-of-scope company %s", dup.ExistingID)
	}

	// And the same conflict against a company the rep CAN see keeps the id, so
	// the "open the existing company" affordance survives for legitimate cases.
	visible, err := e.Contacts.CreateCompany(admin, contacts.CreateCompanyInput{
		DisplayName: "Visible GmbH", Source: "manual",
		Domains: []contacts.CompanyDomainInput{{Domain: "visible-owner.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Contacts.CreateCompany(rep, contacts.CreateCompanyInput{
		DisplayName: "Duplicate attempt 2 GmbH", Source: "manual",
		Domains: []contacts.CompanyDomainInput{{Domain: "visible-owner.test", IsPrimary: true}},
	})
	if !errors.As(err, &dup) {
		t.Fatalf("visible duplicate domain → %v, want contacts.DuplicateDomainError", err)
	}
	if dup.ExistingID != ids.From[ids.CompanyKind](ids.UUID(visible.Id)) {
		t.Errorf("409 for a visible duplicate carries %s, want the owner %s", dup.ExistingID, visible.Id)
	}
}

// repPermsWithActivity extends the rep fixture with activity grants for
// the timeline tests.
func repPermsWithActivity() principal.Permissions {
	p := RepPerms
	objects := make(map[string]principal.ObjectGrant, len(p.Objects)+1)
	for k, v := range p.Objects {
		objects[k] = v
	}
	objects["activity"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	p.Objects = objects
	return p
}

// lostStage resolves the seeded pipeline's lost stage.
func lostStage(t *testing.T, e *Env) ids.StageID {
	t.Helper()
	p, err := e.Deals.DefaultPipeline(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range *p.Stages {
		if st.Semantic == "lost" {
			return ids.From[ids.StageKind](ids.UUID(st.Id))
		}
	}
	t.Fatal("seeded pipeline has no lost stage")
	return ids.StageID{}
}

// A closed deal's frozen FX must track its amount/currency: adding an
// amount to a deal that was closed amountless must freeze a rate (not
// trip deal_closed_fx into a 500), and changing the currency must
// re-freeze as of the CLOSE date (not leave the old currency's rate
// silently corrupting base-currency roll-ups).
func TestRepricingAClosedDealRefreezesFx(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	admin := e.Admin()

	d, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Closed amountless", PipelineID: pipeline, StageID: open, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.AdvanceDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: WonByImport()}); err != nil {
		t.Fatalf("closing amountless: %v", err)
	}

	owner := OwnerConn(t)
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		 VALUES ('USD', 'EUR', 0.9200000000, current_date)`); err != nil {
		t.Fatal(err)
	}

	readFx := func() (rate, date *string) {
		t.Helper()
		if err := owner.QueryRow(context.Background(),
			`SELECT fx_rate_to_base::text, fx_rate_date::text FROM deal WHERE id = $1`,
			ids.UUID(d.Id)).Scan(&rate, &date); err != nil {
			t.Fatal(err)
		}
		return rate, date
	}

	// Adding an amount to the closed deal freezes a rate (base currency → 1).
	amount, eur := int64(48000), "EUR"
	if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.UpdateDealInput{AmountMinor: &amount, Currency: &eur}); err != nil {
		t.Fatalf("adding amount to a closed deal: %v", err)
	}
	rate, _ := readFx()
	if rate == nil {
		t.Fatal("closed deal gained an amount but no frozen FX — deal_closed_fx would have 500ed before the fix")
	}

	// Switching the closed deal's currency re-freezes for the NEW pair. The
	// amount is restated alongside it — the stored numeral carries no unit,
	// so a currency move requires every populated figure to be resent in the
	// same request rather than silently re-denominating 48000 EUR as 48000
	// USD.
	usd := "USD"
	if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)),
		deals.UpdateDealInput{Currency: &usd, AmountMinor: &amount}); err != nil {
		t.Fatalf("re-currencying a closed deal: %v", err)
	}
	rate, _ = readFx()
	if rate == nil || *rate != "0.9200000000" {
		t.Errorf("frozen rate after currency change = %v, want the USD→EUR 0.92 (a stale rate silently corrupts roll-ups)", rate)
	}
}

// Deals are born open: creation directly onto a won/lost stage would put
// an "open" deal on a terminal column with no closed_at/FX — the
// invariant AdvanceDeal exists to maintain, bypassed at birth.
func TestCreateDealRejectsATerminalStage(t *testing.T) {
	e := Setup(t)
	pipeline, _, won := DealFixture(t, e)

	_, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Born won", PipelineID: pipeline, StageID: won, Source: "manual",
	})
	var terminal *deals.TerminalStageOnCreateError
	if !errors.As(err, &terminal) {
		t.Fatalf("create on won stage → %v, want deals.TerminalStageOnCreateError", err)
	}
}

// A client that reopens a lost deal while (redundantly) re-sending the
// lost_reason must not produce a duplicate SET clause — the reopen wins
// and clears the field.
func TestReopeningWithARedundantLostReasonStillCleans(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	lost := lostStage(t, e)
	admin := e.Admin()

	d, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Lost and found", PipelineID: pipeline, StageID: open, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.AdvanceDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.AdvanceDealInput{ToStageID: lost, LostReason: strPtr("price")}); err != nil {
		t.Fatalf("closing as lost: %v", err)
	}
	reopened, err := e.Deals.AdvanceDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.AdvanceDealInput{ToStageID: open, LostReason: strPtr("price")})
	if err != nil {
		t.Fatalf("reopen with redundant lost_reason: %v", err)
	}
	if reopened.LostReason != nil {
		t.Errorf("reopened deal kept lost_reason %q", *reopened.LostReason)
	}
}

// The idempotent-replay path returns a record, so it is a read: replaying
// someone else's external source key must answer a bare conflict when the
// caller may not read that record, never its content. An activity linked
// only to another rep's capture-private contact is such a record. A lead is
// readable by every seat, so a lead replay hands the existing lead back —
// the same answer the caller's own replay would get.
func TestIdempotentReplayDoesNotDiscloseOutOfScopeRecords(t *testing.T) {
	e := Setup(t)
	foreignContact := e.SeedContact(t, "Foreign", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", foreignContact, e.Rep3)
	admin := e.Admin()
	foreignOwner := e.As(e.Rep3, []ids.UUID{e.Team2}, AdminPerms)

	src, key := "gmail", "msg-123"
	if _, _, err := e.Activities.LogActivity(foreignOwner, activities.LogActivityInput{
		Kind: "email", Subject: strPtr("Confidential thread"), Source: "connector",
		SourceSystem: &src, SourceID: &key,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: foreignContact}},
	}); err != nil {
		t.Fatal(err)
	}
	leadSrc, leadKey := "apollo", "lead-9"
	theirLead, _, err := e.Contacts.CreateLead(admin, contacts.CreateLeadInput{
		FullName: strPtr("Foreign lead"), OwnerID: userIDPtr(&e.Rep3), Source: "import",
		SourceSystem: &leadSrc, SourceID: &leadKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, repPermsWithCapture())

	if _, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "email", Source: "connector", SourceSystem: &src, SourceID: &key,
	}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("activity replay of a source key on an unreadable record → %v, want bare ErrConflict", err)
	}
	replayed, _, err := e.Contacts.CreateLead(rep, contacts.CreateLeadInput{
		FullName: strPtr("Replay attempt"), Source: "import",
		SourceSystem: &leadSrc, SourceID: &leadKey,
	})
	if err != nil || replayed.Id != theirLead.Id {
		t.Errorf("lead replay of another team's source key → (%v, %v), want the existing readable lead %v", replayed.Id, err, theirLead.Id)
	}
}

// Link targets are validated against the caller's visibility before the
// insert: the FK alone runs as the table owner and would persist a guessed
// or unreadable UUID as a link. Another team's plain contact is readable and
// so linkable; their capture-private contact is neither.
func TestActivityLinkTargetsMustBeVisible(t *testing.T) {
	e := Setup(t)
	theirContact := e.SeedContact(t, "Their contact", &e.Rep3)
	foreignContact := e.SeedContact(t, "Foreign", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", foreignContact, e.Rep3)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, repPermsWithCapture())

	if _, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "note", Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: theirContact}},
	}); err != nil {
		t.Errorf("link to another team's readable contact → %v, want success", err)
	}
	if _, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "note", Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: foreignContact}},
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("link to a capture-private contact → %v, want ErrNotFound", err)
	}
	if _, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "note", Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.NewV7()}},
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("link to a nonexistent contact → %v, want ErrNotFound", err)
	}
}

// repPermsWithCapture extends the rep fixture with the capture-side
// grants (activity + lead) the replay tests need.
func repPermsWithCapture() principal.Permissions {
	p := repPermsWithActivity()
	objects := make(map[string]principal.ObjectGrant, len(p.Objects)+1)
	for k, v := range p.Objects {
		objects[k] = v
	}
	objects["lead"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	p.Objects = objects
	return p
}

// An account-list filter has ONE input that means "no filter": omitting it.
// Every other value is a selection, and a value outside the contract's enum is
// a client mistake — answered as one, not as an empty page. A 200 with no rows
// tells the reader this workspace has no customers when the question was never
// one the contract accepts.
func TestUnknownAccountFilterValuesAreRefusedRatherThanAnsweredEmpty(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	for _, tc := range []struct {
		name  string
		field string
		in    contacts.ListCompaniesInput
	}{
		{
			"a stage outside the vocabulary", "lifecycle",
			contacts.ListCompaniesInput{Lifecycle: strPtr("nearly_a_customer")},
		},
		{
			"a relationship type outside the vocabulary", "relationship_type",
			contacts.ListCompaniesInput{RelationshipType: strPtr("frenemy")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := e.Contacts.ListCompanies(admin, tc.in)
			var detailed *httperr.DetailedError
			if !errors.As(err, &detailed) {
				t.Fatalf("filter %s=%q → %v, want a validation refusal", tc.field, *strPtrValue(tc.in), err)
			}
			if detailed.Status != http.StatusUnprocessableEntity {
				t.Errorf("filter %s → status %d, want 422", tc.field, detailed.Status)
			}
			if len(detailed.Fields) != 1 || detailed.Fields[0].Field != tc.field {
				t.Errorf("filter %s → fields %+v, want the refusal to name the parameter",
					tc.field, detailed.Fields)
			}
		})
	}

	// The same dials with values the contract DOES define are selections, and
	// answer normally — a rule that refused everything would pass the test
	// above and break the feature.
	for _, in := range []contacts.ListCompaniesInput{
		{Lifecycle: strPtr("customer")},
		{RelationshipType: strPtr("partner")},
	} {
		if _, _, err := e.Contacts.ListCompanies(admin, in); err != nil {
			t.Errorf("a filter value the contract defines was refused: %v", err)
		}
	}
}

// strPtrValue names whichever of the two dials the case set, for the message.
func strPtrValue(in contacts.ListCompaniesInput) *string {
	if in.Lifecycle != nil {
		return in.Lifecycle
	}
	return in.RelationshipType
}
