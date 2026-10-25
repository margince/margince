// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The sender read arm, the fold task and the reply path judge an address at an
// international own domain. Each must use the fold the waiting queue uses.

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// foldSeat is a seat whose login is address, written as identity writes one.
func foldSeat(t *testing.T, e *integration.Env, address string) ids.UUID {
	t.Helper()
	seat := ids.NewV7()
	e.WsExec(t, `INSERT INTO app_user (id, email, display_name, email_domain_folded) VALUES ($1, lower($2), 'Anna', $3)`,
		seat, address, owndomain.StoredOfAddress(address))
	return seat
}

// stampedCopy is a copy in Rep1's mailbox that capture read as sender's mail.
func stampedCopy(t *testing.T, e *integration.Env, sender ids.UUID) ids.UUID {
	t.Helper()
	copyID := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by, sent_by_colleague)
		VALUES ($1, 'email', 'outbound', 'Order form', $2, 'seed', 'connector:gmail', $3)`,
		copyID, clocktest.Now(t).Add(-48*time.Hour), sender)
	return copyID
}

// senderMayHold asks the sender read arm whether seat may hold work from a
// copy on a thread private to Rep1.
func senderMayHold(t *testing.T, e *integration.Env, source, seat ids.UUID) bool {
	t.Helper()
	var may bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		may, err = auth.SeatMayHoldPrivateWorkTx(e.Admin(), tx, source, &e.Rep1, seat)
		return err
	}); err != nil {
		t.Fatalf("asking the sender arm: %v", err)
	}
	return may
}

func runFoldTasks(t *testing.T, e *integration.Env) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	runMachineAddressTask(ctx, t, e, ownDomainFoldDataTask())
	runMachineAddressTask(ctx, t, e, colleagueSenderDataTask())
}

// An administrator types the own domain in unicode and it is stored punycode;
// the colleague's login is unicode. Capture's Go check keeps the stamp, so the
// read arm must open on it, and close once the domain stops being ours.
func TestTheSenderArmFoldsAnOwnDomainTypedInUnicode(t *testing.T) {
	e := integration.Setup(t)
	own := capture.NewOwnDomainStore(InstallationDB(e.Pool))
	if _, err := own.Add(e.Admin(), "Bücher.Example"); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT domain FROM workspace_email_domain`); got != "xn--bcher-kva.example" {
		t.Fatalf("the own domain is stored as %q, want its punycode form", got)
	}
	anna := foldSeat(t, e, "ANNA@BÜCHER.EXAMPLE")
	annaCopy := stampedCopy(t, e, anna)
	runFoldTasks(t, e)

	if !senderMayHold(t, e, annaCopy, anna) {
		t.Fatal("the sender arm refused a colleague capture keeps as ours")
	}
	// Stamped after the recheck ran, so only the read arm can refuse it.
	other := foldSeat(t, e, "anna@bücher-other.example")
	if senderMayHold(t, e, stampedCopy(t, e, other), other) {
		t.Error("the sender arm admitted a seat at a lookalike domain")
	}
	if err := own.Remove(e.Admin(), "bücher.example"); err != nil {
		t.Fatal(err)
	}
	if senderMayHold(t, e, annaCopy, anna) {
		t.Error("the sender arm stayed open after the domain stopped being ours")
	}
}

// The own company claims its domain in unicode, through the company writer,
// and the colleague's login is stored punycode. A subdomain login is ours too,
// as owndomain.Set.Covers says.
func TestTheSenderArmFoldsTheOwnCompanyDomain(t *testing.T) {
	e := integration.Setup(t)
	website := "https://bücher.example"
	if _, err := e.Contacts.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "Bücher GmbH", Website: &website}); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT d.domain FROM company_domain d JOIN company o ON o.id = d.company_id WHERE o.is_anchor`); got != "bücher.example" {
		t.Fatalf("the own company's domain is stored as %q, want the unicode form typed", got)
	}
	for _, address := range []string{"anna@xn--bcher-kva.example", "anna@post.bücher.example"} {
		if !owndomain.New([]string{"bücher.example"}).Covers(address) {
			t.Fatalf("owndomain.Set.Covers(%q) = false; the fixture expects an own address", address)
		}
	}
	anna := foldSeat(t, e, "anna@xn--bcher-kva.example")
	post := foldSeat(t, e, "anna@post.bücher.example")
	annaCopy, postCopy := stampedCopy(t, e, anna), stampedCopy(t, e, post)
	runFoldTasks(t, e)

	if !senderMayHold(t, e, annaCopy, anna) {
		t.Error("the sender arm refused a punycode login at the own company's unicode domain")
	}
	if !senderMayHold(t, e, postCopy, post) {
		t.Error("the sender arm refused a login at a subdomain of the own company's domain")
	}
}

// Rows written before the folded columns existed are read as outsiders until
// the fold task writes them, and then the same as new rows.
func TestTheFoldTaskFoldsRowsWrittenBeforeTheColumns(t *testing.T) {
	e := integration.Setup(t)
	if _, err := capture.NewOwnDomainStore(InstallationDB(e.Pool)).Add(e.Admin(), "bücher.example"); err != nil {
		t.Fatal(err)
	}
	anna := foldSeat(t, e, "anna@bücher.example")
	annaCopy := stampedCopy(t, e, anna)
	participant := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity_participant (id, activity_id, role, address) VALUES ($1, $2, 'from', 'anna@bücher.example')`,
		participant, annaCopy)
	e.WsExec(t, `UPDATE app_user SET email_domain_folded = NULL WHERE id = $1`, anna)
	runMachineAddressTask(principal.SystemActing(e.Admin(), "system:datatask-test"), t, e, colleagueSenderDataTask())
	if senderMayHold(t, e, annaCopy, anna) {
		t.Fatal("the sender arm opened on a login whose fold is not written yet")
	}

	runFoldTasks(t, e)
	if !senderMayHold(t, e, annaCopy, anna) {
		t.Error("the sender arm stayed shut after the fold task wrote the login's domain")
	}
	if got := e.WsScalar(t, `SELECT coalesce(address_domain_folded, '') FROM activity_participant WHERE id = $1`, participant); got != "xn--bcher-kva.example" {
		t.Errorf("the fold task stored %q on the participant, want xn--bcher-kva.example", got)
	}
}

// The reply path names the same colleague for every spelling of her address.
func TestAReplyIsNeverAddressedToAColleagueAtAnInternationalOwnDomain(t *testing.T) {
	e := integration.Setup(t)
	if _, err := capture.NewOwnDomainStore(InstallationDB(e.Pool)).Add(e.Admin(), "bücher.example"); err != nil {
		t.Fatal(err)
	}
	comms := newCommsAdapter(e.Pool, nil, SendPath{})
	for _, colleague := range []string{"anna@bücher.example", "ANNA@BÜCHER.EXAMPLE", "anna@xn--bcher-kva.example"} {
		_, err := comms.ReplyAddress(e.Admin(), seedMeetingWith(t, e, colleague))
		var refusal *activities.NoReplyAddressError
		if !errors.As(err, &refusal) || !refusal.Colleague {
			t.Errorf("ReplyAddress to %s → %v, want the colleague refusal", colleague, err)
		}
	}
	got, err := comms.ReplyAddress(e.Admin(), seedMeetingWith(t, e, "anna@bücher-other.example"))
	if err != nil || got != "anna@bücher-other.example" {
		t.Fatalf("ReplyAddress → (%q, %v), want the guest at the lookalike domain", got, err)
	}
}
