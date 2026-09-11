// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The consumer's whole reason to exist is ordering: a company's team page is
// read once, and the contacts it published are staged as proposals. A contact who
// arrives AFTERWARDS was never matched against what that page already said.
// This suite drives that arrival — a contact.created envelope — and asserts the
// contact ends up carrying the page's facts with the page as their receipt.
//
// The proposals are staged through approvals.Service, the same writer
// siteleadstage.go uses, rather than inserted by hand: a row shape the real
// writer never produces would let the consumer pass against a fixture that
// cannot occur.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sitePage is what a team page published about one contact, in the shape the
// staged proposal carries it.
type sitePage struct {
	CompanyID       ids.UUID `json:"company_id"`
	SiteReadID      ids.UUID `json:"site_read_id"`
	NaturalKey      string   `json:"natural_key"`
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	PublishedEmail  string   `json:"published_email,omitempty"`
	LinkedinURL     string   `json:"linkedin_url,omitempty"`
	EvidenceSnippet string   `json:"evidence_snippet"`
	SourceURL       string   `json:"source_url"`
}

// seedEmployedContact creates a contact and the current-primary employment that
// makes their employer's site the only one allowed to describe them.
func seedEmployedContact(t *testing.T, e *Env, name string) (ids.ContactID, ids.CompanyID) {
	t.Helper()
	contactID, companyID := ids.NewV7(), ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO company (id, owner_id, display_name, source, captured_by)
			VALUES ($1, $2, 'Gitex', 'manual', 'user:seed')`, companyID, e.Rep1); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO contact (id, owner_id, full_name, source, captured_by)
			VALUES ($1, $2, $3, 'manual', 'user:seed')`,
			contactID, e.Rep1, name); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO relationship (id, contact_id, company_id, kind,
			                          is_current_primary, source, captured_by)
			VALUES ($1, $2, $3, 'employment', true, 'manual', 'user:seed')`,
			ids.NewV7(), contactID, companyID)
		return err
	}); err != nil {
		t.Fatalf("seeding the employed contact: %v", err)
	}
	return ids.From[ids.ContactKind](contactID), ids.From[ids.CompanyKind](companyID)
}

// stageSiteLead stages one published contact exactly as the site-read lane does.
func stageSiteLead(t *testing.T, e *Env, companyID ids.CompanyID, page sitePage) ids.ApprovalID {
	t.Helper()
	page.CompanyID = companyID.UUID
	page.SiteReadID = ids.NewV7()
	page.NaturalKey = page.Name
	proposed, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshalling the proposal: %v", err)
	}
	identity, err := json.Marshal(map[string]string{"natural_key": page.NaturalKey})
	if err != nil {
		t.Fatalf("marshalling the identity: %v", err)
	}
	digest := sha256.Sum256(proposed)
	id, err := approvals.NewService(e.DB()).Stage(e.Admin(), approvals.StageInput{
		Kind:           "site_lead",
		ProposedChange: proposed,
		DiffHash:       hex.EncodeToString(digest[:]),
		TargetType:     "company",
		TargetID:       companyID.UUID,
		Identity:       identity,
		JoinPending:    true,
		Summary:        "Lead from the team page: " + page.Name,
	})
	if err != nil {
		t.Fatalf("staging the site lead: %v", err)
	}
	return id
}

// newEnricher builds the consumer with no search provider — the sovereign
// posture of ADR-0081, and the arm this suite exercises. Discovery has its own
// unit tests against a fake client; what is under test here is the fill from
// what the employer already published.
func newEnricher(e *Env) *compose.ContactAutoEnrich {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	return compose.NewContactAutoEnrich(e.Pool, contacts.NewStore(e.DB()), approvals.NewService(e.DB()), nil, quiet)
}

// contactCreated is the envelope a newly created contact reaches the consumer on.
//
// The trace is not decoration: the pass carries the correlation id through to
// whatever it writes, so the fill traces back to the event that caused it, and
// the outbox refuses an envelope without one.
func contactCreated(contactID ids.ContactID) events.Envelope {
	return events.Envelope{
		Type:   "contact.created",
		Entity: events.EntityRef{Type: "contact", ID: contactID.UUID},
		Trace:  events.Trace{CorrelationID: ids.NewV7()},
	}
}

// profileFields reads back what the pass wrote, keyed by field name.
func profileFields(t *testing.T, e *Env, contactID ids.ContactID) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(),
			`SELECT field, value FROM contact_profile_field WHERE contact_id = $1`, contactID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var field, value string
			if err := rows.Scan(&field, &value); err != nil {
				return err
			}
			out[field] = value
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the profile fields: %v", err)
	}
	return out
}

// A contact who arrives after their employer's page was read must still end up
// carrying what that page said. This is the ordering bug the consumer exists to
// close: the site read matched nobody at the time, and nothing would ever look
// again.
func TestContactAutoEnrichFillsAContactFromTheirEmployersStagedPage(t *testing.T) {
	e := Setup(t)
	contactID, companyID := seedEmployedContact(t, e, "Anna Muster")
	approvalID := stageSiteLead(t, e, companyID, sitePage{
		Name:            "Anna Muster",
		Role:            "Head of Delivery",
		LinkedinURL:     "https://www.linkedin.com/in/annamuster",
		EvidenceSnippet: "Anna Muster — Head of Delivery",
		SourceURL:       "https://gitex.com/team",
	})

	if err := newEnricher(e).HandleEvent(context.Background(), contactCreated(contactID)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	fields := profileFields(t, e, contactID)
	if got := fields["role"]; got != "Head of Delivery" {
		t.Errorf("role = %q, want the role the page published", got)
	}
	if got := fields["linkedin"]; got != "https://www.linkedin.com/in/annamuster" {
		t.Errorf("linkedin = %q, want the URL the page published", got)
	}

	// The proposal asked a human to create a lead for someone who is already a
	// contact. The world answered the question, so it must leave the queue.
	//
	// Withdrawal expires the row rather than moving its status, so asserting on
	// `status` would pass against a proposal still sitting in a rep's inbox.
	// The comparison is against the row's own creation time rather than the
	// clock: an expiry that moved BEHIND the moment the row was staged is
	// withdrawal, and reading that off `now()` would make the assertion depend
	// on how long the test took.
	var withdrawn bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT expires_at < created_at FROM approval WHERE id = $1`, approvalID).Scan(&withdrawn)
	}); err != nil {
		t.Fatalf("reading the proposal back: %v", err)
	}
	if !withdrawn {
		t.Error("the site-lead proposal is still live, so a rep is asked to create a lead for a contact that exists")
	}
}

// The match is narrow on purpose. A page contact who is not unmistakably this
// contact must leave the record alone rather than be settled by a sweep.
func TestContactAutoEnrichLeavesAContactAloneWhenThePageNamesSomebodyElse(t *testing.T) {
	e := Setup(t)
	contactID, companyID := seedEmployedContact(t, e, "Anna Muster")
	stageSiteLead(t, e, companyID, sitePage{
		Name:            "Bernd Schulz",
		Role:            "Head of Procurement",
		EvidenceSnippet: "Bernd Schulz — Head of Procurement",
		SourceURL:       "https://gitex.com/team",
	})

	if err := newEnricher(e).HandleEvent(context.Background(), contactCreated(contactID)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if fields := profileFields(t, e, contactID); len(fields) != 0 {
		t.Errorf("the contact carries %v, but the page named a different contact", fields)
	}
}

// No employer means no site may describe this contact, and the pass must stop
// before it reads anything. A contact with a staged page at a company they do
// not work for is the case that would silently cross records.
//
// Another company's page names this same contact, so the employer gate is what
// the assertion rests on. Without that page the test would pass on an empty
// database and prove nothing.
func TestContactAutoEnrichStopsAtAContactWithNoEmployer(t *testing.T) {
	e := Setup(t)
	_, foreignCompany := seedEmployedContact(t, e, "Somebody Else")
	stageSiteLead(t, e, foreignCompany, sitePage{
		Name:            "Anna Muster",
		Role:            "Head of Delivery",
		EvidenceSnippet: "Anna Muster — Head of Delivery",
		SourceURL:       "https://elsewhere.test/team",
	})

	contactID := ids.From[ids.ContactKind](ids.NewV7())
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO contact (id, owner_id, full_name, source, captured_by)
			VALUES ($1, $2, 'Anna Muster', 'manual', 'user:seed')`, contactID, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding the unemployed contact: %v", err)
	}

	if err := newEnricher(e).HandleEvent(context.Background(), contactCreated(contactID)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if fields := profileFields(t, e, contactID); len(fields) != 0 {
		t.Errorf("the contact carries %v with no employer to have published it", fields)
	}
}

// A merge names the merged-AWAY contact as its entity. Enriching that row fills a
// record no read returns, and the survivor — which is the one that actually
// became newly matchable, inheriting the source's emails and employer — would be
// missed entirely.
func TestContactAutoEnrichFollowsAMergeToTheSurvivor(t *testing.T) {
	e := Setup(t)
	survivor, companyID := seedEmployedContact(t, e, "Anna Muster")
	mergedAway := ids.From[ids.ContactKind](ids.NewV7())
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO contact (id, owner_id, full_name, source, captured_by, archived_at)
			VALUES ($1, $2, 'A. Muster', 'manual', 'user:seed', now())`, mergedAway, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding the merged-away contact: %v", err)
	}
	stageSiteLead(t, e, companyID, sitePage{
		Name:            "Anna Muster",
		Role:            "Head of Delivery",
		EvidenceSnippet: "Anna Muster — Head of Delivery",
		SourceURL:       "https://gitex.com/team",
	})

	payload, err := json.Marshal(map[string]string{"merged_into_id": survivor.String()})
	if err != nil {
		t.Fatalf("marshalling the merge payload: %v", err)
	}
	if err := newEnricher(e).HandleEvent(context.Background(), events.Envelope{
		Type:    "contact.merged",
		Entity:  events.EntityRef{Type: "contact", ID: mergedAway.UUID},
		Trace:   events.Trace{CorrelationID: ids.NewV7()},
		Payload: payload,
	}); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if got := profileFields(t, e, survivor)["role"]; got != "Head of Delivery" {
		t.Errorf("the survivor's role = %q, want the page's — the merge went to the archived row", got)
	}
	if fields := profileFields(t, e, mergedAway); len(fields) != 0 {
		t.Errorf("the merged-away row carries %v, which no read will ever return", fields)
	}
}
