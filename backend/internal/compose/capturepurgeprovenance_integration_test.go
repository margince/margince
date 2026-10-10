// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Which contacts an owner's sender purge may anonymise: only the ones capture
// made for that owner and nobody worked on since. Every other contact keeps its
// name and address, even when its mail goes.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The rule every test here purges, and the addresses it covers.
const (
	purgeDomain   = "kanzlei.example"
	typedAddress  = "anwalt@kanzlei.example"
	mintedAddress = "assistenz@kanzlei.example"
)

func TestASenderPurgeKeepsAContactTheOwnerTypedIn(t *testing.T) {
	e := integration.Setup(t)
	connectRep1Mailbox(t, e)
	const typedName = "Dr. Kanzlei Hand"

	// Typed in by the owner, with one address and no deal; then mail arrives
	// from it into the owner's mailbox.
	owner := ids.From[ids.UserKind](e.Rep1)
	typed, err := e.Contacts.CreateContact(purgeCtx(e, e.Rep1), contacts.CreateContactInput{
		FullName: typedName,
		OwnerID:  &owner,
		Emails:   []contacts.ContactEmailInput{{Email: typedAddress, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("typing in the contact: %v", err)
	}
	typedMail := seedPurgeableMail(t, e, typedAddress, "Mandat", e.Rep1)
	minted := mintThroughCapture(t, e, "typed")

	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, purgeDomain)
	preview := runPurge(t, e, e.Rep1, rule, true)
	outcome := runPurge(t, e, e.Rep1, rule, false)

	// The preview and the receipt count only the contact the purge touches.
	if preview.Anonymised != 1 || outcome.Anonymised != 1 {
		t.Fatalf("anonymised: preview=%d purge=%d, want 1 and 1 — only the mailbox's own contact",
			preview.Anonymised, outcome.Anonymised)
	}
	if body := activityBody(t, e, typedMail); body != "" {
		t.Fatalf("the typed contact's mail kept its body %q; the rule is about mail", body)
	}
	if contactIDFor(t, e, typedAddress).UUID != ids.UUID(typed.Id) {
		t.Fatal("the typed contact lost its address to a mail cleanup")
	}
	if name, archived := contactNameAndArchived(t, e, ids.UUID(typed.Id)); name != typedName || archived {
		t.Fatalf("the typed contact reads name=%q archived=%v, want %q and live", name, archived, typedName)
	}
	if _, archived := contactNameAndArchived(t, e, minted); !archived {
		t.Fatal("the contact the mailbox created survived the purge of its sender")
	}
}

// An API caller created the contact. Its history holds no human row, so only
// the record's origin tells it apart from one capture made.
func TestASenderPurgeKeepsAContactAnAgentCreated(t *testing.T) {
	e := integration.Setup(t)
	connectRep1Mailbox(t, e)
	owner := ids.From[ids.UserKind](e.Rep1)
	created, err := e.Contacts.CreateContact(ownerAgentCtx(e), contacts.CreateContactInput{
		FullName: "Dr. Kanzlei API",
		OwnerID:  &owner,
		Emails:   []contacts.ContactEmailInput{{Email: typedAddress, EmailType: "work", IsPrimary: true}},
		Source:   "api",
	})
	if err != nil {
		t.Fatalf("creating the contact through the API: %v", err)
	}
	seedPurgeableMail(t, e, typedAddress, "Mandat", e.Rep1)

	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, purgeDomain)
	if outcome := runPurge(t, e, e.Rep1, rule, false); outcome.Anonymised != 0 {
		t.Fatalf("anonymised %d, want 0: an API caller made this contact", outcome.Anonymised)
	}
	if _, archived := contactNameAndArchived(t, e, ids.UUID(created.Id)); archived {
		t.Fatal("a contact an API caller created was erased by a mail cleanup")
	}
}

// Capture made the contact, then a human worked on it. The stamp still names
// the connector, so only the record's later history can tell.
func TestASenderPurgeKeepsACapturedContactSomebodyWorkedOn(t *testing.T) {
	cases := map[string]func(t *testing.T, e *integration.Env, minted ids.UUID){
		"an edit by an agent": func(t *testing.T, e *integration.Env, minted ids.UUID) {
			title := "Kanzleileitung"
			if _, err := e.Contacts.UpdateContact(ownerAgentCtx(e), ids.From[ids.ContactKind](minted),
				contacts.UpdateContactInput{Title: &title}); err != nil {
				t.Fatalf("editing the contact: %v", err)
			}
		},
		"a merge of a typed contact into it, by an agent": func(t *testing.T, e *integration.Env, minted ids.UUID) {
			typed, err := e.Contacts.CreateContact(ownerCtx(e), contacts.CreateContactInput{
				FullName: "Dr. Kanzlei Hand",
				Emails:   []contacts.ContactEmailInput{{Email: typedAddress, EmailType: "work", IsPrimary: true}},
				Source:   "manual",
			})
			if err != nil {
				t.Fatalf("typing in the contact: %v", err)
			}
			if _, err := e.Contacts.MergeContact(ownerAgentCtx(e), ids.From[ids.ContactKind](ids.UUID(typed.Id)),
				ids.From[ids.ContactKind](minted), nil); err != nil {
				t.Fatalf("merging the typed contact into the captured one: %v", err)
			}
		},
		"a promotion of a typed lead into it": func(t *testing.T, e *integration.Env, minted ids.UUID) {
			name, email, title := "Assistenz Kanzlei", mintedAddress, "Partnerin"
			lead, _, err := e.Contacts.CreateLead(ownerCtx(e), contacts.CreateLeadInput{
				FullName: &name, Email: &email, Title: &title, Source: "manual",
			})
			if err != nil {
				t.Fatalf("typing in the lead: %v", err)
			}
			promoted, _, err := e.Contacts.PromoteLead(ownerCtx(e), ids.From[ids.LeadKind](ids.UUID(lead.Id)),
				contacts.PromoteLeadInput{Trigger: string(contacts.TriggerHumanQualify)})
			if err != nil {
				t.Fatalf("promoting the lead: %v", err)
			}
			if ids.UUID(promoted.Id) != minted {
				t.Fatalf("the lead was promoted into %s, want the captured contact %s", promoted.Id, minted)
			}
		},
	}
	for name, workOn := range cases {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			connectRep1Mailbox(t, e)
			minted := mintThroughCapture(t, e, "worked")
			workOn(t, e, minted)

			rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, purgeDomain)
			if outcome := runPurge(t, e, e.Rep1, rule, false); outcome.Anonymised != 0 {
				t.Fatalf("anonymised %d, want 0: after %s the contact is not only what the mailbox said",
					outcome.Anonymised, name)
			}
			if _, archived := contactNameAndArchived(t, e, minted); archived {
				t.Fatalf("the contact was erased after %s", name)
			}
		})
	}
}

// A human worked on a record that names the contact rather than on the contact
// row itself. The contact's own history shows nothing, so the link rows must.
func TestASenderPurgeKeepsACapturedContactARelatedRecordNames(t *testing.T) {
	cases := map[string]func(t *testing.T, e *integration.Env, minted ids.UUID){
		"a tag": func(t *testing.T, e *integration.Env, minted ids.UUID) {
			tags := collections.NewStore(e.DB())
			tag, err := tags.CreateTag(ownerCtx(e), "Mandanten", nil, nil)
			if err != nil {
				t.Fatalf("creating the tag: %v", err)
			}
			if _, err := tags.ApplyTag(ownerCtx(e), tag.ID, "contact", minted); err != nil {
				t.Fatalf("tagging the contact: %v", err)
			}
		},
		"a list": func(t *testing.T, e *integration.Env, minted ids.UUID) {
			lists := collections.NewStore(e.DB())
			list, err := lists.CreateList(ownerCtx(e), collections.CreateListInput{Name: "Kanzleien", EntityType: "contact"})
			if err != nil {
				t.Fatalf("creating the list: %v", err)
			}
			if _, err := lists.AddMember(ownerCtx(e), list.ID,
				collections.MemberChange{EntityType: "contact", EntityID: minted, Reason: "chosen"}); err != nil {
				t.Fatalf("adding the contact to the list: %v", err)
			}
		},
		"a relationship": func(t *testing.T, e *integration.Env, minted ids.UUID) {
			company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Kanzlei GmbH", &e.Rep1))
			contact := ids.From[ids.ContactKind](minted)
			if _, err := e.Contacts.CreateRelationship(ownerCtx(e), contacts.CreateRelationshipInput{
				Kind: "employment", ContactID: &contact, CompanyID: &company,
			}); err != nil {
				t.Fatalf("relating the contact to a company: %v", err)
			}
		},
		"a note": func(t *testing.T, e *integration.Env, minted ids.UUID) {
			body := "Rückruf vereinbart"
			if _, _, err := e.Activities.LogActivity(ownerCtx(e), activities.LogActivityInput{
				Kind: "note", Body: &body, Source: "manual",
				Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: minted}},
			}); err != nil {
				t.Fatalf("writing a note on the contact: %v", err)
			}
		},
	}
	for name, workOn := range cases {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			connectRep1Mailbox(t, e)
			minted := mintThroughCapture(t, e, "related")
			workOn(t, e, minted)

			rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, purgeDomain)
			if outcome := runPurge(t, e, e.Rep1, rule, false); outcome.Anonymised != 0 {
				t.Fatalf("anonymised %d, want 0: %s names the contact", outcome.Anonymised, name)
			}
			if _, archived := contactNameAndArchived(t, e, minted); archived {
				t.Fatalf("the contact was erased although %s names it", name)
			}
		})
	}
}

// Somebody edits the contact after the purge chose it and before the scrub
// reached it. The scrub asks again and keeps the contact, and the receipt
// counts nothing.
func TestASenderPurgeRechecksAContactBeforeAnonymisingIt(t *testing.T) {
	e := integration.Setup(t)
	connectRep1Mailbox(t, e)
	minted := mintThroughCapture(t, e, "recheck")
	ruleID := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, purgeDomain)
	purger := purgerFor(t, e)
	ctx := purgeCtx(e, e.Rep1)
	rule, _, err := purger.purgeableRule(ctx, ruleID, e.Rep1)
	if err != nil {
		t.Fatalf("reading the rule: %v", err)
	}
	chosen, err := purger.purgeableContacts(ctx, e.Rep1, rule)
	if err != nil || len(chosen) != 1 {
		t.Fatalf("the purge chose %v (err=%v), want the captured contact", chosen, err)
	}

	title := "Kanzleileitung"
	if _, err := e.Contacts.UpdateContact(ownerCtx(e), ids.From[ids.ContactKind](minted),
		contacts.UpdateContactInput{Title: &title}); err != nil {
		t.Fatalf("editing the contact: %v", err)
	}
	anonymised, err := purger.carryOut(ctx, capture.PurgeSubject{}, chosen,
		purger.stillPurgeable(e.Rep1, rule), e.Rep1, privacy.PurgeOwnerRule)
	if err != nil {
		t.Fatalf("carrying out the purge: %v", err)
	}
	if anonymised != 0 {
		t.Fatalf("anonymised %d, want 0: the contact was edited after it was chosen", anonymised)
	}
	if _, archived := contactNameAndArchived(t, e, minted); archived {
		t.Fatal("a contact edited after the purge chose it was erased anyway")
	}
}

// The purge keeps a message under a hold, so it keeps the sender's contact too.
// The retained message still needs to say who wrote it.
func TestASenderPurgeKeepsTheContactOfMailItMustRetain(t *testing.T) {
	e := integration.Setup(t)
	connectRep1Mailbox(t, e)
	minted := mintThroughCapture(t, e, "retained")
	restrict(t, e, capturedMailFrom(t, e, mintedAddress))

	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, purgeDomain)
	outcome := runPurge(t, e, e.Rep1, rule, false)
	if outcome.Skipped != 1 || outcome.Anonymised != 0 {
		t.Fatalf("skipped=%d anonymised=%d, want 1 and 0", outcome.Skipped, outcome.Anonymised)
	}
	if _, archived := contactNameAndArchived(t, e, minted); archived {
		t.Fatal("the contact of a message under a hold was erased")
	}
}

// capturedMailFrom returns the inbound message capture imported for Rep1 from
// address.
func capturedMailFrom(t *testing.T, e *integration.Env, address string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT a.id FROM activity a JOIN capture_import i ON i.activity_id = a.id
			 WHERE i.user_id = $1 AND a.counterparty_email = $2`, e.Rep1, address).Scan(&id)
	}); err != nil {
		t.Fatalf("finding the captured mail from %s: %v", address, err)
	}
	return id
}

// The verdict pass creates the contact capture withheld while the sender was
// unjudged. That contact is capture's as much as one the sink made.
func TestASenderPurgeAnonymisesAContactTheVerdictCreated(t *testing.T) {
	e := integration.Setup(t)
	connectRep1Mailbox(t, e)
	mail := seedPurgeableMail(t, e, mintedAddress, "Termin", e.Rep1)
	verdictCtx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), verdictActor)
	if err := database.WithWorkspaceTx(verdictCtx, e.Pool, func(tx pgx.Tx) error {
		_, err := createCounterpartyRecords(verdictCtx, tx, e.Contacts, nil, counterpartyCreation{
			Email: mintedAddress, DisplayName: "Assistenz", Domain: purgeDomain,
			OwnerID: e.Rep1, ActivityID: mail, Source: verdictReason, CapturedBy: verdictActor,
		})
		return err
	}); err != nil {
		t.Fatalf("creating the contact the verdict earned: %v", err)
	}
	minted := contactIDFor(t, e, mintedAddress).UUID

	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, purgeDomain)
	if outcome := runPurge(t, e, e.Rep1, rule, false); outcome.Anonymised != 1 {
		t.Fatalf("anonymised %d, want 1: the verdict's contact is capture's", outcome.Anonymised)
	}
	if _, archived := contactNameAndArchived(t, e, minted); !archived {
		t.Fatal("the contact the verdict created survived the purge of its sender")
	}
}

// Both seats connect Gmail, so the contact's stamp fits either. The contact is
// still only the seat's whose mailbox made it.
func TestASenderPurgeLeavesAContactAnotherSeatsMailboxCreated(t *testing.T) {
	e := integration.Setup(t)
	connectRep1Mailbox(t, e)
	seedMailboxConnection(t, e, e.Rep2, "gmail", "b@authz.test")
	minted := mintThroughCapture(t, e, "seats")
	// Rep1 lets go of the mail, and Rep2's mailbox is now the only one holding
	// mail from that address.
	releaseEveryImportOf(t, e, mintedAddress, e.Rep1)
	seedPurgeableMail(t, e, mintedAddress, "Weitergeleitet", e.Rep2)

	rule := seedOwnExclusion(t, e, e.Rep2, capture.ExclusionKindDomain, purgeDomain)
	if outcome := runPurge(t, e, e.Rep2, rule, false); outcome.Anonymised != 0 {
		t.Fatalf("anonymised %d, want 0: the contact is Rep1's mailbox's, not Rep2's", outcome.Anonymised)
	}
	if _, archived := contactNameAndArchived(t, e, minted); archived {
		t.Fatal("one seat's purge erased a contact another seat's mailbox created")
	}
}

// ownerCtx is Rep1 working by hand on the records their mailbox brought in,
// which capture keeps visible to Rep1 alone.
func ownerCtx(e *integration.Env) context.Context {
	return ownerCtxAs(e, principal.PrincipalHuman, "human:"+e.Rep1.String())
}

// ownerAgentCtx is an agent acting for Rep1, as an API caller with a passport
// does. Its audit rows say agent, not human.
func ownerAgentCtx(e *integration.Env) context.Context {
	return ownerCtxAs(e, principal.PrincipalAgent, "agent:assistant")
}

func ownerCtxAs(e *integration.Env, kind principal.PrincipalType, actorID string) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	grant := principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	objects := map[string]principal.ObjectGrant{}
	for _, object := range []string{"contact", "lead", "activity", "company", "relationship", "tag", "list"} {
		objects[object] = grant
	}
	return principal.WithActor(ctx, principal.Principal{
		Type: kind, ID: actorID, UserID: e.Rep1, OnBehalfOf: e.Rep1,
		SeatType:    principal.SeatFull,
		Permissions: principal.Permissions{Objects: objects, RowScope: principal.RowScopeAll},
	})
}

// connectRep1Mailbox gives Rep1 a Gmail connection labelled with the login
// address captureInboundThroughRealSink delivers to.
func connectRep1Mailbox(t *testing.T, e *integration.Env) {
	t.Helper()
	seedMailboxConnection(t, e, e.Rep1, "gmail", "a@authz.test")
}

// mintThroughCapture lets Rep1's mailbox create a contact for mintedAddress. The
// workspace writes first, and the production sink captures the reply and
// creates the contact.
func mintThroughCapture(t *testing.T, e *integration.Env, key string) ids.UUID {
	t.Helper()
	seedAttestedOutbound(t, e, key+"-out", mintedAddress, key+"-thread")
	captureInboundThroughRealSink(t, e, e.Rep1, key+"-in", mintedAddress, key+"-thread")
	minted := contactIDFor(t, e, mintedAddress).UUID
	if by := contactCapturedBy(t, e, minted); !strings.HasPrefix(by, "connector:") {
		t.Fatalf("the sink created a contact stamped captured_by=%q, want the mailbox connector", by)
	}
	return minted
}

// seedMailboxConnection records that a seat connected a mailbox: the row
// capture's own connect writes. Capture reads its address to tell that the seat
// received a message.
func seedMailboxConnection(t *testing.T, e *integration.Env, user ids.UUID, provider, address string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label)
			VALUES ($1, $2, 'connected', 'test-credential', $3)`, user, provider, address)
		return err
	}); err != nil {
		t.Fatalf("connecting the %s mailbox: %v", provider, err)
	}
}

// releaseEveryImportOf drops one seat's claim on every message from address.
// It goes through the writer a shared-message purge uses.
func releaseEveryImportOf(t *testing.T, e *integration.Env, address string, user ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT a.id FROM activity a JOIN capture_import i ON i.activity_id = a.id
			 WHERE i.user_id = $1 AND a.counterparty_email = $2`, user, address)
		if err != nil {
			return err
		}
		held, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return err
		}
		for _, id := range held {
			if err := capture.ReleaseImportTx(context.Background(), tx, id, user); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("releasing the seat's imports: %v", err)
	}
}

func contactCapturedBy(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	var by string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(captured_by, '') FROM contact WHERE id = $1`, id).Scan(&by)
	}); err != nil {
		t.Fatalf("reading who created contact %s: %v", id, err)
	}
	return by
}

func contactNameAndArchived(t *testing.T, e *integration.Env, id ids.UUID) (string, bool) {
	t.Helper()
	var name string
	var archived bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(full_name, ''), archived_at IS NOT NULL FROM contact WHERE id = $1`,
			id).Scan(&name, &archived)
	}); err != nil {
		t.Fatalf("reading contact %s: %v", id, err)
	}
	return name, archived
}

// anonymiseAsChosen keeps every contact a test hands AnonymiseContacts: those
// tests are about the scrub, not about who may be scrubbed.
func anonymiseAsChosen(context.Context, pgx.Tx, ids.UUID) (bool, error) { return true, nil }
