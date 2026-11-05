// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type partyMailProof struct{ outbound, sentByOwner bool }

func capturePartiesMail(t *testing.T, e *integration.Env, proof partyMailProof, cc, bcc, body string) datasource.EntityRef {
	t.Helper()
	return capturePartiesMailUsing(t, e, newCaptureSink(e.Pool, CaptureConfig{}), proof, cc, bcc, body)
}

func capturePartiesMailUsing(t *testing.T, e *integration.Env, sink *capture.Sink, proof partyMailProof, cc, bcc, body string) datasource.EntityRef {
	t.Helper()
	rec := partiesMailRecord(t, e, proof, cc, bcc, body)
	ctx := threadConnectorCtx(e, e.AdminUser)
	ref, err := sink.Upsert(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("capture replay: %v", err)
	}
	return ref
}

func TestCapturedPartiesRecoveryAdvancesPastAnEarlierActivity(t *testing.T) {
	e := integration.Setup(t)
	activityOnly := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil)
	first := capturePartiesMailUsing(t, e, activityOnly, partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	second := capturePartiesMailUsing(t, e, activityOnly, partyMailProof{outbound: true, sentByOwner: true}, "Casey Buyer <casey@buyer.test>", "", "Could we discuss the product requirements?")
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email) LIKE '%@buyer.test'`) != 0 {
		t.Fatal("activity-only capture unexpectedly created contacts")
	}
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	system := principal.SystemActing(e.Admin(), "party-recovery-test")
	if err := sink.RecoverPartyAdmissions(system, 1); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND lower(address)='bob@buyer.test' AND contact_id IS NOT NULL`, first.ID) != 1 {
		t.Fatal("the first recovery page did not file its copied recipient")
	}
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='casey@buyer.test'`) != 0 {
		t.Fatal("one recovery page exceeded its activity bound")
	}
	if err := sink.RecoverPartyAdmissions(system, 1); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND lower(address)='casey@buyer.test' AND contact_id IS NOT NULL`, second.ID) != 1 {
		t.Fatal("the durable cursor did not reach the later activity")
	}
	if err := sink.RecoverPartyAdmissions(system, 1); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='contact' AND action='create'`) != 3 {
		t.Fatal("recovery repeated contact creation")
	}
}

func TestCapturedPartiesAdmitsEveryTrustedToAndCcOnlyOnce(t *testing.T) {
	e := integration.Setup(t)
	ref := capturePartiesMail(t, e, partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <BOB@buyer.test>", "Hidden Buyer <hidden@buyer.test>", "Could we discuss the product requirements?")
	for _, address := range []string{"alice@buyer.test", "bob@buyer.test"} {
		if n := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)=$1 AND archived_at IS NULL`, address); n != 1 {
			t.Fatalf("%s has %d contacts", address, n)
		}
		if n := e.WsCount(t, `SELECT count(*) FROM activity_participant p JOIN contact_email ce ON ce.contact_id=p.contact_id WHERE p.activity_id=$1 AND lower(ce.email)=$2`, ref.ID, address); n != 1 {
			t.Fatalf("%s was not promoted on the activity: %d", address, n)
		}
	}
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='hidden@buyer.test'`) != 0 {
		t.Fatal("a blind-copied party was admitted")
	}
	if e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='contact' AND action='create'`) != 2 {
		t.Fatal("capture replay repeated contact creation")
	}
}

func TestCapturedPartiesLeavesUntrustedAndDecliningMailAsEvidence(t *testing.T) {
	for _, tc := range []struct {
		name               string
		outbound, attested bool
		body               string
	}{
		{"inbound recipient", false, false, "Please meet my colleague."},
		{"unattested own From", true, false, "Please meet my colleague."},
		{"declining first send", true, true, "We are not interested. Please remove us."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			ref := capturePartiesMail(t, e, partyMailProof{outbound: tc.outbound, sentByOwner: tc.attested}, "Bob Buyer <bob@buyer.test>", "", tc.body)
			if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email) IN ('alice@buyer.test','bob@buyer.test')`) != 0 {
				t.Fatal("untrusted or declining mail admitted a contact")
			}
			if e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND lower(address)='bob@buyer.test'`, ref.ID) != 1 {
				t.Fatal("the copied party's evidence was lost")
			}
		})
	}
}

func TestCapturedPartiesKeepsLiveLeadsInTheLeadPool(t *testing.T) {
	e := integration.Setup(t)
	email := "bob@buyer.test"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Email: &email, FullName: new("Bob Buyer"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	ref := capturePartiesMail(t, e, partyMailProof{outbound: true, sentByOwner: true}, fmt.Sprintf("Bob Buyer <%s>", email), "", "Could we discuss the product requirements?")
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)=$1`, email) != 0 {
		t.Fatal("a copied recipient's lead was converted")
	}
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND lead_id=$2`, ref.ID, lead.Id) != 1 {
		t.Fatal("the copied recipient's activity did not reach their lead")
	}
}

func TestCapturedPartiesStopsCreatingContactsWhenTheirActivityHasNoRoom(t *testing.T) {
	e := integration.Setup(t)
	var copied []string
	for index := range 30 {
		copied = append(copied, fmt.Sprintf("Buyer %d <buyer%d@buyer.test>", index, index))
	}
	ref := capturePartiesMail(t, e, partyMailProof{outbound: true, sentByOwner: true}, strings.Join(copied, ", "), "", "Could we discuss the product requirements?")
	created := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email) LIKE '%@buyer.test' AND archived_at IS NULL`)
	filed := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND entity_type='contact'`, ref.ID)
	if created != filed || created < 2 || created >= len(copied) {
		t.Fatalf("created=%d filed=%d recipients=%d", created, filed, len(copied)+1)
	}
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1`, ref.ID) > 25 {
		t.Fatal("participant admission exceeded the activity filing limit")
	}
	if e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND role='cc' AND contact_id IS NULL`, ref.ID) == 0 {
		t.Fatal("unadmitted copied-recipient evidence was lost")
	}
}

func TestCapturedPartiesRecoveryRechecksTheCapturingSeatsCurrentAuthority(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	service := identity.NewServiceFor(e.DB())
	activatePartySeat(t, e, e.AdminUser)
	capturePartiesMailUsing(t, e, newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil), partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	e.GrantRole(t, e.Rep1, "admin")
	rbac, seat, err := service.EffectiveAuthority(e.Admin(), e.WS, e.Rep1)
	if err != nil {
		t.Fatal(err)
	}
	admin := identity.Identity{UserID: ids.From[ids.UserKind](e.Rep1), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS), SeatType: string(seat), Roles: rbac.Permissions.RoleKeys, Permissions: rbac.Permissions}
	ctx := e.As(e.Rep1, rbac.TeamIDs, rbac.Permissions)
	owner := ids.From[ids.UserKind](e.AdminUser)
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithPartyAuthority(&afterPartyAuthorityRead{
		PartyAuthority: service,
		after: func() {
			if err := service.DeactivateUser(ctx, admin, identity.DeactivateUserInput{UserID: owner}); err != nil {
				t.Fatal(err)
			}
		},
	})
	system := principal.SystemActing(e.Admin(), "party-recovery-test")
	if err := sink.RecoverPartyAdmissions(system, 200); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`) != 0 {
		t.Fatal("a seat deactivated after the authority read still admitted a recipient")
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE user_id=$1 AND admission_pending`, e.AdminUser) != 1 {
		t.Fatal("revoked authority discarded durable pending work")
	}
	if err := service.ReactivateUser(ctx, admin, owner); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := sink.RecoverPartyAdmissions(system, 200); err != nil {
			t.Fatal(err)
		}
	}
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`) != 1 {
		t.Fatal("restored live authority did not allow durable recovery")
	}
}

func partiesMailRecord(t *testing.T, e *integration.Env, proof partyMailProof, cc, bcc, body string) connector.NormalizedRecord {
	t.Helper()
	e.GrantRole(t, e.AdminUser, "admin")
	owner := seatAddress(t, e, e.AdminUser)
	from, to := owner, "Alice Buyer <alice@buyer.test>"
	if !proof.outbound {
		from, to = "Alice Buyer <alice@buyer.test>", owner
	}
	lines := []string{"From: " + from, "To: " + to, "Subject: Product discussion", "Date: Mon, 5 Oct 2026 10:00:00 +0000", "Message-ID: <" + ids.NewV7().String() + "@parties.test>", "Content-Type: text/plain; charset=utf-8"}
	if cc != "" {
		lines = append(lines, "Cc: "+cc)
	}
	if bcc != "" {
		lines = append(lines, "Bcc: "+bcc)
	}
	lines = append(lines, "", body, "")
	raw := []byte(strings.Join(lines, "\r\n"))
	parsed, err := mailmap.Parse(raw, owner)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.AttestSentByOwner(proof.sentByOwner).ToRecord("gmail", raw)
}

func TestCapturedPartiesRetiresEarlierAuthorityWhenTheFolderIsExcluded(t *testing.T) {
	e := integration.Setup(t)
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	rec := partiesMailRecord(t, e, partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	rec.Containers = []string{"gmail:SENT"}
	ctx := threadConnectorCtx(e, e.AdminUser)
	ref, err := sink.WithEnsurer(nil, nil).Upsert(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capture.NewExclusionStore(InstallationDB(e.Pool)).Add(e.Admin(), capture.ExclusionScopeUser, capture.ExclusionKindContainer, "gmail:Private"); err != nil {
		t.Fatal(err)
	}
	rec.Containers = []string{"gmail:SENT", "gmail:Private"}
	drop, err := sink.DropBeforeStore(ctx, rec)
	if err != nil || !drop {
		t.Fatalf("drop=%v: %v", drop, err)
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND admission_blocked`, ref.ID) != 1 {
		t.Fatal("earlier source authority survived exclusion")
	}
	if err := sink.RecoverPartyAdmissions(principal.SystemActing(e.Admin(), "party-recovery-test"), 10); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`) != 0 {
		t.Fatal("excluded source admitted a recipient")
	}
}

func TestCapturedPartiesHonorsAnExclusionOnTheCapturingAddress(t *testing.T) {
	e := integration.Setup(t)
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	capturePartiesMailUsing(t, e, sink.WithEnsurer(nil, nil), partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	if _, err := capture.NewExclusionStore(InstallationDB(e.Pool)).Add(e.Admin(), capture.ExclusionScopeUser, capture.ExclusionKindAddress, seatAddress(t, e, e.AdminUser)); err != nil {
		t.Fatal(err)
	}
	if err := sink.RecoverPartyAdmissions(principal.SystemActing(e.Admin(), "party-recovery-test"), 10); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`) != 0 {
		t.Fatal("an excluded user address lost its policy effect when resolved to a seat")
	}
}

func TestCapturedPartiesCannotUseAnArchivedSendAsAdmissionProof(t *testing.T) {
	e := integration.Setup(t)
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	ref := capturePartiesMailUsing(t, e, sink.WithEnsurer(nil, nil), partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](ref.ID), nil); err != nil {
		t.Fatal(err)
	}
	rec := partiesMailRecord(t, e, partyMailProof{outbound: false, sentByOwner: false}, "", "", "Thanks for the information.")
	parsed, err := mailmap.Parse([]byte(strings.ReplaceAll(string(rec.Raw), "Alice Buyer <alice@buyer.test>", "Bob Buyer <bob@buyer.test>")), seatAddress(t, e, e.AdminUser))
	if err != nil {
		t.Fatal(err)
	}
	rec = parsed.ToRecord("gmail", rec.Raw)
	if _, err := sink.Upsert(threadConnectorCtx(e, e.AdminUser), rec); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`) != 0 {
		t.Fatal("archived correspondence minted a contact")
	}
}

func partyOwnerContext(t *testing.T, e *integration.Env) context.Context {
	t.Helper()
	authority := identity.NewService(e.Pool)
	rbac, err := authority.EffectiveRBAC(e.Admin(), e.WS, e.AdminUser)
	if err != nil {
		t.Fatal(err)
	}
	seat, err := authority.SeatType(e.Admin(), e.WS, e.AdminUser)
	if err != nil {
		t.Fatal(err)
	}
	return principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.AdminUser.String(), UserID: e.AdminUser, OnBehalfOf: e.AdminUser, Permissions: rbac.Permissions, TeamIDs: rbac.TeamIDs, SeatType: seat})
}

func activatePartySeat(t *testing.T, e *integration.Env, user ids.UUID) {
	t.Helper()
	if err := e.DB().Tx(principal.SystemActing(e.Admin(), "party-owner-setup"), func(tx pgx.Tx) error {
		var email string
		if err := tx.QueryRow(e.Admin(), `SELECT email FROM app_user WHERE id=$1`, user).Scan(&email); err != nil {
			return err
		}
		return identity.OperatorResetPassword(e.Admin(), tx, ids.From[ids.WorkspaceKind](e.WS), email, "temporary test phrase")
	}); err != nil {
		t.Fatal(err)
	}
}
