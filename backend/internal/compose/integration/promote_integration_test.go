// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The features/01 §6.4 acceptance criteria for lead→contact promotion:
// non-lossy graduation carrying provenance, merge-not-duplicate via the
// §1.3 email path, the one-transaction audit+event shape, and the scope
// rules a merge inherits from being a read.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func seedLead(t *testing.T, e *Env, name, email string, owner *ids.UUID) ids.LeadID {
	t.Helper()
	in := contacts.CreateLeadInput{Source: "import", OwnerID: userIDPtr(owner)}
	if name != "" {
		in.FullName = &name
	}
	if email != "" {
		in.Email = &email
	}
	l, _, err := e.Contacts.CreateLead(e.Admin(), in)
	if err != nil {
		t.Fatalf("seeding lead %s: %v", name, err)
	}
	return leadIDOf(ids.UUID(l.Id))
}

func TestPromoteCreatesAContactCarryingProvenance(t *testing.T) {
	e := Setup(t)
	leadID := seedLead(t, e, "Ada Prospect", "ada@prospect.test", &e.Rep1)
	admin := e.Admin()

	contact, merged, err := e.Contacts.PromoteLead(admin, leadID, contacts.PromoteLeadInput{
		Trigger: "inbound_reply", EvidenceNote: strPtr("replied to outreach"),
	})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if merged {
		t.Error("fresh email should create, not merge")
	}
	if contact.ConvertedFromLeadId == nil || leadIDOf(ids.UUID(*contact.ConvertedFromLeadId)) != leadID {
		t.Error("contact lost the converted_from_lead_id origin pointer")
	}
	if contact.OwnerId == nil || ids.UUID(*contact.OwnerId) != e.Rep1 {
		t.Error("promotion dropped the lead's owner")
	}
	if contact.Source != "import" {
		t.Errorf("promotion rewrote provenance source to %q; the capture channel must survive", contact.Source)
	}
	if contact.Emails == nil || len(*contact.Emails) != 1 || string((*contact.Emails)[0].Email) != "ada@prospect.test" {
		t.Error("promotion lost the lead's email")
	}

	// The lead is graduated: promoted, stamped with the outcome, archived
	// off the lead list — but still resolvable by id for the audit trail.
	lead, err := e.Contacts.GetLead(admin, leadID, storekit.IncludeArchived)
	if err != nil {
		t.Fatal(err)
	}
	if string(lead.Status) != "promoted" || lead.PromotedContactId == nil || lead.ArchivedAt == nil {
		t.Errorf("lead after promote: status=%s promoted_contact_id=%v archived_at=%v", lead.Status, lead.PromotedContactId, lead.ArchivedAt)
	}

	// Exactly one lead.promoted with the §5.5 payload, plus the caused
	// contact.created — same correlation, same audit row.
	owner := OwnerConn(t)
	var payload json.RawMessage
	var promotedAudit, contactAudit string
	if err := owner.QueryRow(context.Background(),
		`SELECT envelope->'payload', envelope->'trace'->>'audit_log_id' FROM event_outbox
		 WHERE envelope->>'type' = 'lead.promoted'`).Scan(&payload, &promotedAudit); err != nil {
		t.Fatalf("lead.promoted not staged: %v", err)
	}
	var p struct {
		PromotedContactID ids.UUID `json:"promoted_contact_id"`
		DedupeOutcome     string   `json:"dedupe_outcome"`
		Trigger           string   `json:"trigger"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.PromotedContactID != ids.UUID(contact.Id) || p.DedupeOutcome != "created" || p.Trigger != "inbound_reply" {
		t.Errorf("lead.promoted payload %s", payload)
	}
	if err := owner.QueryRow(context.Background(),
		`SELECT envelope->'trace'->>'audit_log_id' FROM event_outbox
		 WHERE envelope->>'type' = 'contact.created' AND envelope->'entity'->>'id' = $1`,
		contact.Id.String()).Scan(&contactAudit); err != nil {
		t.Fatalf("contact.created not staged: %v", err)
	}
	if promotedAudit != contactAudit {
		t.Error("promotion split across audit rows; the spec demands one transaction, one audit entry")
	}

	// Promotion happens once: the replay answers the typed 409 with the
	// outcome pointer, never a second contact.
	_, _, err = e.Contacts.PromoteLead(admin, leadID, contacts.PromoteLeadInput{Trigger: "human_qualify"})
	var already *contacts.AlreadyPromotedError
	if !errors.As(err, &already) {
		t.Fatalf("re-promote → %v, want contacts.AlreadyPromotedError", err)
	}
	if already.ContactID != ContactIDOf(ids.UUID(contact.Id)) {
		t.Error("409 lost the promoted_contact_id pointer")
	}
}

func TestPromoteMergesIntoAnExistingContactNotADuplicate(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	existing, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{
		FullName: "Grace Known", OwnerID: userIDPtr(&e.Rep1), Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "grace@known.test", EmailType: "work", IsPrimary: true, Position: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	leadID := seedLead(t, e, "G. Known", "grace@known.test", &e.Rep2)

	contact, merged, err := e.Contacts.PromoteLead(admin, leadID, contacts.PromoteLeadInput{Trigger: "meeting_booked"})
	if err != nil {
		t.Fatalf("promote-with-match: %v", err)
	}
	if !merged || ids.UUID(contact.Id) != ids.UUID(existing.Id) {
		t.Fatalf("merged=%v into %s, want merge into the one existing contact %s", merged, contact.Id, existing.Id)
	}
	if contact.ConvertedFromLeadId == nil || leadIDOf(ids.UUID(*contact.ConvertedFromLeadId)) != leadID {
		t.Error("merge did not record the lead origin")
	}
	if contact.FullName != "Grace Known" {
		t.Errorf("merge overwrote the human-curated name with %q", contact.FullName)
	}

	owner := OwnerConn(t)
	var contacts int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		 WHERE pe.email = 'grace@known.test' AND p.archived_at IS NULL`).Scan(&contacts); err != nil {
		t.Fatal(err)
	}
	if contacts != 1 {
		t.Fatalf("%d live contacts hold the email after promotion, want exactly 1 (merge-not-duplicate)", contacts)
	}
}

func TestPromoteDoesNotDiscloseAnOutOfScopeMergeTarget(t *testing.T) {
	e := Setup(t)
	if _, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Foreign Match", OwnerID: userIDPtr(&e.Rep3), Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "match@foreign.test", EmailType: "work", IsPrimary: true, Position: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	leadID := seedLead(t, e, "Mine", "match@foreign.test", &e.Rep1)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, repPermsWithCapture())
	if _, _, err := e.Contacts.PromoteLead(rep, leadID, contacts.PromoteLeadInput{Trigger: "inbound_reply"}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("promote into an out-of-scope match → %v, want bare ErrConflict (a merge is a read)", err)
	}
}

func TestPromoteRequiresBothLeadAndContactGrants(t *testing.T) {
	e := Setup(t)
	leadID := seedLead(t, e, "Gated", "gated@x.test", &e.Rep1)

	// Lead grants but no contact.create: leads may be worked, contacts may
	// not be minted through the promotion door.
	perms := repPermsWithCapture()
	perms.Objects["contact"] = principal.ObjectGrant{Read: true, Update: true}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	if _, _, err := e.Contacts.PromoteLead(rep, leadID, contacts.PromoteLeadInput{Trigger: "human_qualify"}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("promote without contact.create → %v, want ErrPermissionDenied", err)
	}
}
