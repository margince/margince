// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func retainedSignatureFixture(t *testing.T) (*integration.Env, *contacts.Store, ids.UUID) {
	t.Helper()
	e := integration.Setup(t)
	memberManagerFor(t, e)
	store := contacts.NewStore(InstallationDB(e.Pool)).WithPrivacyReviewer(identity.NewServiceFor(InstallationDB(e.Pool)))
	if _, err := store.SaveMyEmailSignature(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms), contacts.SaveSignatureInput{Body: "Original personal text"}); err != nil {
		t.Fatal(err)
	}
	var id ids.UUID
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT id FROM email_signature WHERE owner_id=$1`, e.Rep1).Scan(&id)
	}); err != nil {
		t.Fatal(err)
	}
	return e, store, id
}

func TestARetainedSignatureNeedsAHumanControllerAndAnInactiveOwner(t *testing.T) {
	e, store, id := retainedSignatureFixture(t)
	if _, err := store.GetRetainedSignature(e.Admin(), id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("active owner's signature: %v", err)
	}
	deactivateSignatureOwner(t, e)
	got, err := store.GetRetainedSignature(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.OwnerID != e.Rep1 || got.ID != id || got.Signature.Body != "Original personal text" {
		t.Fatalf("reviewed the wrong signature: %+v", got)
	}
	if body := e.WsScalar(t, `SELECT body FROM email_signature WHERE id=$1`, id); body != "Original personal text" {
		t.Fatal("opening review changed the original")
	}
	if _, err := contacts.NewStore(InstallationDB(e.Pool)).GetRetainedSignature(e.Admin(), id); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("no reviewer configured: %v", err)
	}
	actor, ok := principal.Actor(e.Admin())
	if !ok {
		t.Fatal("missing admin principal")
	}
	actor.Type = principal.PrincipalAgent
	if _, err := store.GetRetainedSignature(principal.WithActor(e.Admin(), actor), id); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("agent read: %v", err)
	}
	revokeSignatureController(t, e)
	if _, err := store.GetRetainedSignature(e.Admin(), id); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("revoked stored authority: %v", err)
	}
}

func TestAControllerSavesTheExactRetainedSignatureAgainstItsVersion(t *testing.T) {
	e, store, id := retainedSignatureFixture(t)
	if _, err := store.SaveMyEmailSignature(e.Admin(), contacts.SaveSignatureInput{Body: "Controller's own text"}); err != nil {
		t.Fatal(err)
	}
	deactivateSignatureOwner(t, e)
	e.WsExec(t, `UPDATE email_signature SET archived_at=now() WHERE id=$1`, id)
	before, err := store.GetRetainedSignature(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.SaveRetainedSignature(e.Admin(), id, before.Version, contacts.SaveSignatureInput{Body: "Reviewed text"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != id || saved.OwnerID != e.Rep1 || saved.Signature.Body != "Reviewed text" || saved.Version <= before.Version {
		t.Fatalf("saved: %+v", saved)
	}
	if _, err := store.SaveRetainedSignature(e.Admin(), id, before.Version, contacts.SaveSignatureInput{Body: "Stale overwrite"}); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale save: %v", err)
	}
	if got := e.WsScalar(t, `SELECT body FROM email_signature WHERE owner_id=$1`, e.AdminUser); got != "Controller's own text" {
		t.Fatal("review changed the controller's own signature")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM email_signature WHERE id=$1 AND archived_at IS NOT NULL AND body='Reviewed text'`, id); n != 1 {
		t.Fatal("review resurrected or replaced the archived signature")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid=a.id WHERE a.entity_type='user' AND a.entity_id=$1 AND a.actor_id=$2 AND a.after->>'has_signature'='true' AND o.envelope->>'type'='email_signature.changed'`, e.Rep1, "human:"+e.AdminUser.String()); n != 1 {
		t.Fatalf("controller audit and outbox pairs: %d", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid=a.id
 WHERE a.entity_type='user' AND a.entity_id=$1
 AND (a.before::text LIKE '%Original personal text%' OR a.after::text LIKE '%Reviewed text%' OR o.envelope::text LIKE '%Reviewed text%')`, e.Rep1); n != 0 {
		t.Fatalf("signature text escaped into %d audit/outbox pairs", n)
	}

	returnSignatureOwner(t, e)
	if _, err := store.SaveRetainedSignature(e.Admin(), id, saved.Version, contacts.SaveSignatureInput{Body: "After reactivation"}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reactivated owner's signature: %v", err)
	}
}

func TestTheControllerReviewRouteNamesTheRetainedSignature(t *testing.T) {
	e, store, id := retainedSignatureFixture(t)
	deactivateSignatureOwner(t, e)
	shown, err := store.GetRetainedSignature(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	server := crmcontracts.Handler(Server{contactsHandlers: newContactsHandlers(e.Pool)})
	path := "/privacy/retained-signatures/" + id.String()
	read := httptest.NewRecorder()
	server.ServeHTTP(read, httptest.NewRequest(http.MethodGet, path, nil).WithContext(e.Admin()))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), id.String()) || !strings.Contains(read.Body.String(), e.Rep1.String()) || !strings.Contains(read.Body.String(), "Original personal text") {
		t.Fatalf("review read: %d %s", read.Code, read.Body.String())
	}
	missing := httptest.NewRecorder()
	server.ServeHTTP(missing, httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"body":"Changed"}`)).WithContext(e.Admin()))
	if missing.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing version: %d %s", missing.Code, missing.Body.String())
	}
	save := httptest.NewRecorder()
	body := fmt.Sprintf(`{"body":"Reviewed over HTTP","expected_version":%d}`, shown.Version)
	server.ServeHTTP(save, httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)).WithContext(e.Admin()))
	if save.Code != http.StatusOK || !strings.Contains(save.Body.String(), "Reviewed over HTTP") {
		t.Fatalf("review save: %d %s", save.Code, save.Body.String())
	}
	stale := httptest.NewRecorder()
	server.ServeHTTP(stale, httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)).WithContext(e.Admin()))
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "version_skew") {
		t.Fatalf("stale HTTP save: %d %s", stale.Code, stale.Body.String())
	}
	denied := httptest.NewRecorder()
	server.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, path, nil).WithContext(e.As(e.Rep2, nil, integration.SchedulerPerms)))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("non-controller HTTP read: %d %s", denied.Code, denied.Body.String())
	}
	got, err := store.GetRetainedSignature(e.Admin(), id)
	if err != nil || got.Signature.Body != "Reviewed over HTTP" {
		t.Fatalf("stored review: %+v, %v", got, err)
	}
}

func TestTheRetainedSignaturePreviewUsesTheMailSanitizer(t *testing.T) {
	e, _, id := retainedSignatureFixture(t)
	markup := `<p>Retained words</p><img src="https://tracker.test/pixel"><script>window.alert(1)</script>`
	if _, err := contacts.NewStore(InstallationDB(e.Pool)).SaveMyEmailSignature(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms), contacts.SaveSignatureInput{Body: "Original personal text", HTML: &markup}); err != nil {
		t.Fatal(err)
	}
	deactivateSignatureOwner(t, e)
	read := httptest.NewRecorder()
	server := crmcontracts.Handler(Server{contactsHandlers: newContactsHandlers(e.Pool)})
	server.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/privacy/retained-signatures/"+id.String(), nil).WithContext(e.Admin()))
	var signature crmcontracts.RetainedEmailSignature
	if err := json.Unmarshal(read.Body.Bytes(), &signature); err != nil {
		t.Fatal(err)
	}
	if read.Code != http.StatusOK || signature.Signature.Html != markup {
		t.Fatalf("the stored source was changed: %d %+v", read.Code, signature)
	}
	if signature.PreviewHtml == nil || !strings.Contains(*signature.PreviewHtml, "Retained words") || strings.Contains(*signature.PreviewHtml, "tracker.test") || strings.Contains(*signature.PreviewHtml, "<script") {
		t.Fatalf("unsafe or absent preview: %+v", signature.PreviewHtml)
	}
}

func TestAReadSeatCannotReceiveOrOpenARetainedSignatureReview(t *testing.T) {
	e, store, id := retainedSignatureFixture(t)
	deactivateSignatureOwner(t, e)
	e.WsExec(t, `UPDATE app_user SET seat_type='read' WHERE id=$1`, e.AdminUser)
	if _, err := store.GetRetainedSignature(e.Admin(), id); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("stored read seat was admitted: %v", err)
	}
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		recipients, err := identity.PrivacyReviewOperatorsTx(principal.SystemActing(e.Admin(), "system:privacy_review_test"), tx, "contact")
		if err != nil {
			return err
		}
		if len(recipients) != 0 {
			t.Fatalf("read seat received a privacy-controller notice: %v", recipients)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestARetainedSignatureSaveOrdersTheMembersReturnAfterItsReview(t *testing.T) {
	e, store, id := retainedSignatureFixture(t)
	ctx := e.Admin()
	deactivateSignatureOwner(t, e)
	shown, err := store.GetRetainedSignature(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	members := identity.NewServiceFor(InstallationDB(e.Pool))
	administrator, err := members.MemberAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		rbac, seat, err := members.EffectiveAuthorityTx(ctx, tx, e.AdminUser)
		if err != nil {
			return err
		}
		administrator.Permissions, administrator.SeatType = rbac.Permissions, string(seat)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	saved, restored := make(chan error, 1), make(chan error, 1)
	if err := e.DB().Tx(ctx, func(holder pgx.Tx) error {
		var locked ids.UUID
		if err := holder.QueryRow(ctx, `SELECT id FROM email_signature WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
			return err
		}
		if locked != id {
			t.Fatal("locked a different signature")
		}
		go func() {
			_, err := store.SaveRetainedSignature(ctx, id, shown.Version, contacts.SaveSignatureInput{Body: "Reviewed before the return"})
			saved <- err
		}()
		reviewer := waitForBackendBlockedBy(t, holder, backendPID(t, holder), saved)
		go func() {
			restored <- members.ReactivateUser(ctx, administrator, ids.From[ids.UserKind](e.Rep1))
		}()
		waitForBackendBlockedBy(t, holder, reviewer, restored)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Fatalf("saving the review: %v", err)
	}
	if err := <-restored; err != nil {
		t.Fatalf("returning the member: %v", err)
	}
	if got := e.WsScalar(t, `SELECT body FROM email_signature WHERE id=$1`, id); got != "Reviewed before the return" {
		t.Fatalf("the committed review was lost: %s", got)
	}
	if got := e.WsScalar(t, `SELECT status FROM app_user WHERE id=$1`, e.Rep1); got == "deactivated" {
		t.Fatal("the member did not return after the review committed")
	}
}

func TestAControllerCanReviewSignaturesNoActiveHumanOwns(t *testing.T) {
	for _, state := range []string{"invited", "archived", "agent"} {
		t.Run(state, func(t *testing.T) {
			e, store, id := retainedSignatureFixture(t)
			owner := e.Rep1
			switch state {
			case "invited":
				deactivateSignatureOwner(t, e)
				signatureOwnerInvitation(t, e)
			case "archived":
				// Human retirement writes removed_at; archived human rows are retained legacy data.
				e.WsExec(t, `UPDATE app_user SET archived_at=now() WHERE id=$1`, e.Rep1)
			case "agent":
				id, owner = serviceMemberSignature(t, e, store)
			}
			got, err := store.GetRetainedSignature(e.Admin(), id)
			if err != nil {
				t.Fatal(err)
			}
			if got.OwnerID != owner || got.Signature.Body != "Original personal text" {
				t.Fatalf("read another owner's text: %+v", got)
			}
		})
	}
}

func serviceMemberSignature(t *testing.T, e *integration.Env, store *contacts.Store) (ids.UUID, ids.UUID) {
	t.Helper()
	member := declareSignatureServiceMember(t, e)
	actor := principal.Principal{Type: principal.PrincipalAgent, ID: "agent:" + member.String(), UserID: member.UUID, SeatType: principal.SeatFull}
	if _, err := store.SaveMyEmailSignature(principal.WithActor(e.Admin(), actor), contacts.SaveSignatureInput{Body: "Original personal text"}); err != nil {
		t.Fatal(err)
	}
	var id ids.UUID
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT id FROM email_signature WHERE owner_id=$1`, member).Scan(&id)
	}); err != nil {
		t.Fatal(err)
	}
	return id, member.UUID
}

func declareSignatureServiceMember(t *testing.T, e *integration.Env) ids.UserID {
	t.Helper()
	members := identity.NewServiceFor(e.DB())
	company := contacts.NewStore(e.DB()).WithInstallationRenamer(installationRenamer(e.Pool))
	if _, err := company.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "Signature review company"}); err != nil {
		t.Fatal(err)
	}
	identity.NewHandlers(members).WithInstallationDescribed(installationDescribed(company))
	member, err := members.DeclareServiceMember(e.Admin(), recoveryAdminIdentity(e), identity.ServiceMemberInput{Extension: "signature-review", DisplayName: "Signature service"})
	if err != nil {
		t.Fatal(err)
	}
	return member
}

func deactivateSignatureOwner(t *testing.T, e *integration.Env) {
	t.Helper()
	members := newMemberManager(e.DB()).service
	if err := members.DeactivateUser(e.Admin(), recoveryAdminIdentity(e), identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](e.Rep1)}); err != nil {
		t.Fatal(err)
	}
}

func signatureOwnerInvitation(t *testing.T, e *integration.Env) (*identity.Service, string) {
	t.Helper()
	members := newMemberManager(e.DB()).service
	admin := recoveryAdminIdentity(e)
	if err := members.ReactivateUser(e.Admin(), admin, ids.From[ids.UserKind](e.Rep1)); err != nil {
		t.Fatal(err)
	}
	token, _, err := members.IssuePasswordLink(e.Admin(), admin, ids.From[ids.UserKind](e.Rep1))
	if err != nil {
		t.Fatal(err)
	}
	return members, token
}

func returnSignatureOwner(t *testing.T, e *integration.Env) {
	t.Helper()
	members, token := signatureOwnerInvitation(t, e)
	if err := members.RedeemPasswordReset(e.Admin(), token, "Retained-owner-chooses-42!"); err != nil {
		t.Fatal(err)
	}
}

func revokeSignatureController(t *testing.T, e *integration.Env) {
	t.Helper()
	members := newMemberManager(e.DB()).service
	admin := recoveryAdminIdentity(e)
	if err := members.ChangeUserRole(e.Admin(), admin, ids.From[ids.UserKind](e.Rep2), "admin"); err != nil {
		t.Fatal(err)
	}
	other := admin
	other.UserID = ids.From[ids.UserKind](e.Rep2)
	if err := members.ChangeUserRole(e.As(e.Rep2, nil, integration.AdminPerms), other, admin.UserID, "rep"); err != nil {
		t.Fatal(err)
	}
}

func TestInvitationActivationWaitsForTheRetainedSignatureReview(t *testing.T) {
	e, store, id := retainedSignatureFixture(t)
	deactivateSignatureOwner(t, e)
	members, token := signatureOwnerInvitation(t, e)
	ctx := e.Admin()
	shown, err := store.GetRetainedSignature(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	saved, activated := make(chan error, 1), make(chan error, 1)
	if err := e.DB().Tx(ctx, func(holder pgx.Tx) error {
		var locked ids.UUID
		if err := holder.QueryRow(ctx, `SELECT id FROM email_signature WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
			return err
		}
		if locked != id {
			t.Fatal("locked another signature")
		}
		go func() {
			_, err := store.SaveRetainedSignature(ctx, id, shown.Version, contacts.SaveSignatureInput{Body: "Reviewed before activation"})
			saved <- err
		}()
		reviewer := waitForBackendBlockedBy(t, holder, backendPID(t, holder), saved)
		go func() { activated <- members.RedeemPasswordReset(ctx, token, "Retained-owner-chooses-42!") }()
		waitForBackendBlockedBy(t, holder, reviewer, activated)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Fatalf("saving the review: %v", err)
	}
	if err := <-activated; err != nil {
		t.Fatalf("activating the owner: %v", err)
	}
	if status := e.WsScalar(t, `SELECT status FROM app_user WHERE id=$1`, e.Rep1); status != "active" {
		t.Fatalf("owner status: %s", status)
	}
	if _, err := store.GetRetainedSignature(ctx, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("active owner remained reviewable: %v", err)
	}
	if body := e.WsScalar(t, `SELECT body FROM email_signature WHERE id=$1`, id); body != "Reviewed before activation" {
		t.Fatalf("review lost: %s", body)
	}
}
