// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A restriction of processing (GDPR Art. 18) keeps the AI off every mail the
// restricted contact sends or receives. The mail stays stored, and the AI reads
// it once the restriction is lifted.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const restrictedAddress = "anna.weber@restricted.example"

// restrictContact creates a contact holding restrictedAddress and records a
// restriction on it through the consent stop writer.
func restrictContact(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Anna Weber", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: restrictedAddress, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	anna := ids.UUID(created.Id)
	if err := consent.NewStore(InstallationDB(e.Pool)).Suppress(e.Admin(), consent.SuppressInput{
		ContactID: ids.From[ids.ContactKind](anna), Kind: "processing_restriction",
	}); err != nil {
		t.Fatalf("recording the restriction: %v", err)
	}
	return anna
}

// mailFromAddress is an unlabelled email with one sender, named by address only,
// as capture stores it before the sender is linked to a contact.
func mailFromAddress(t *testing.T, e *integration.Env, subject, address string) ids.UUID {
	t.Helper()
	id := seedUnlabeledEmail(t, e, subject)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'from')`, id, address)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func liftRestrictionDirectly(t *testing.T, e *integration.Env, contact ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE communication_suppression SET revoked_at = now()
			 WHERE contact_id = $1 AND kind = 'processing_restriction'`, contact)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTheClassifierSkipsMailOfARestrictedContactUntilTheLift(t *testing.T) {
	e := integration.Setup(t)
	anna := restrictContact(t, e)
	restricted := mailFromAddress(t, e, "about my disputed address", "Anna.Weber@restricted.example")
	ordinary := mailFromAddress(t, e, "please send the offer", "bob@elsewhere.example")
	brain := &scriptedClassifyBrain{}
	classifier := NewCaptureClassifier(e.Pool, brain, slog.New(slog.DiscardHandler))
	pass := principal.WithWorkspaceID(context.Background(), e.WS)

	if err := classifier.RunWorkspace(pass, 0); err != nil {
		t.Fatalf("a pass beside a restricted mail must not fail: %v", err)
	}
	if labelOf(t, e, ordinary) == nil {
		t.Fatal("the ordinary mail was not labelled")
	}
	if label := labelOf(t, e, restricted); label != nil {
		t.Fatalf("the restricted contact's mail was read and labelled %q", *label)
	}

	liftRestrictionDirectly(t, e, anna)
	if err := classifier.RunWorkspace(pass, 0); err != nil {
		t.Fatal(err)
	}
	if labelOf(t, e, restricted) == nil {
		t.Fatal("after the lift the mail that arrived during the restriction was not read")
	}
}

func TestTheRouterCheckFindsARestrictedContactByIdAddressAndMail(t *testing.T) {
	e := integration.Setup(t)
	anna := restrictContact(t, e)
	restricted := mailFromAddress(t, e, "about my disputed address", restrictedAddress)
	ordinary := mailFromAddress(t, e, "please send the offer", "bob@elsewhere.example")
	check := subjectRestrictions{pool: e.Pool}
	ctx := e.Admin()

	for name, tc := range map[string]struct {
		subjects model.Subjects
		want     bool
	}{
		"the contact":          {model.Subjects{Contacts: []ids.UUID{anna}}, true},
		"an address it holds":  {model.Subjects{Addresses: []string{"ANNA.WEBER@restricted.example"}}, true},
		"mail it sent":         {model.Subjects{Activities: []ids.UUID{ordinary, restricted}}, true},
		"mail it is not on":    {model.Subjects{Activities: []ids.UUID{ordinary}}, false},
		"another contact":      {model.Subjects{Contacts: []ids.UUID{ids.NewV7()}}, false},
		"no subjects at all":   {model.Subjects{}, false},
		"an unrelated address": {model.Subjects{Addresses: []string{"bob@elsewhere.example"}}, false},
	} {
		got, err := check.AnyRestricted(ctx, tc.subjects)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != tc.want {
			t.Errorf("%s: restricted = %v, want %v", name, got, tc.want)
		}
	}

	liftRestrictionDirectly(t, e, anna)
	if got, err := check.AnyRestricted(ctx, model.Subjects{Contacts: []ids.UUID{anna}}); err != nil || got {
		t.Fatalf("a lifted restriction still restricts: %v %v", got, err)
	}
}

// A revoked buyer is no longer in the room brief's buyer list. What they wrote
// while they had a seat can still be in it, so the room names them.
func TestTheRouterCheckReachesARestrictedContactThroughTheirCompanyDealAndRoom(t *testing.T) {
	e := integration.Setup(t)
	anna := restrictContact(t, e)
	company := e.SeedCompany(t, "Brandt Automotive", nil)
	deal := e.SeedWonDealLinkedTo(t)
	room := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
			VALUES ('employment', $1, $2, 'manual', 'human:x')`, anna, company); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO deal_room (id, deal_id, title, source, captured_by)
			VALUES ($1, $2, 'Room', 'manual', 'human:x')`, room, deal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO deal_room_participant (room_id, email, full_name, source, captured_by, revoked_at)
			VALUES ($1, $2, 'Anna Weber', 'manual', 'human:x', now())`, room, restrictedAddress)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	check := subjectRestrictions{pool: e.Pool}
	for name, subjects := range map[string]model.Subjects{
		"the company she works for":         {Companies: []ids.UUID{company}},
		"a room where her seat was revoked": {Rooms: []ids.UUID{room}},
	} {
		got, err := check.AnyRestricted(e.Admin(), subjects)
		if err != nil || !got {
			t.Errorf("%s: restricted = %v (%v), want true", name, got, err)
		}
	}
	if got, err := check.AnyRestricted(e.Admin(), model.Subjects{Companies: []ids.UUID{e.SeedCompany(t, "Other", nil)}}); err != nil || got {
		t.Errorf("a company she has nothing to do with: restricted = %v (%v)", got, err)
	}
}

// An agent's tools read activity content through the one content gate, so an
// agent reads no mail a restricted contact sent or received.
func TestAnAgentReadsNoMailOfARestrictedContact(t *testing.T) {
	e := integration.Setup(t)
	restrictContact(t, e)
	restricted := mailFromAddress(t, e, "about my disputed address", restrictedAddress)
	ordinary := mailFromAddress(t, e, "please send the offer", "bob@elsewhere.example")
	actor, ok := principal.Actor(e.Admin())
	if !ok {
		t.Fatal("the admin context carries no principal")
	}
	actor.Type, actor.ID, actor.PassportID, actor.OnBehalfOf = principal.PrincipalAgent, "agent:test", ids.NewV7(), actor.UserID
	agent := principal.WithActor(e.Admin(), actor)

	var readable []ids.UUID
	if err := database.WithWorkspaceTx(agent, e.Pool, func(tx pgx.Tx) error {
		args := []any{[]ids.UUID{restricted, ordinary}}
		clause, err := auth.ActivityContentClause(agent, "a", func(v any) int { args = append(args, v); return len(args) })
		if err != nil {
			return err
		}
		rows, err := tx.Query(context.Background(), `SELECT a.id FROM activity a WHERE a.id = ANY($1) AND `+clause, args...)
		if err != nil {
			return err
		}
		readable, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(readable) != 1 || readable[0] != ordinary {
		t.Fatalf("an agent may read %v, want only the ordinary mail %v", readable, ordinary)
	}
}

// The same proof through real tool calls, as an MCP client makes them. The
// mail is filed on a company the agent may read, so it stays discoverable. Only
// the restriction keeps its subject out of the answer.
func TestAnAgentToolCallReturnsNoRestrictedContactOrTheirMail(t *testing.T) {
	e := integration.Setup(t)
	anna := restrictContact(t, e)
	restricted := mailFromAddress(t, e, "about my disputed address", restrictedAddress)
	ordinary := mailFromAddress(t, e, "please send the offer", "bob@elsewhere.example")
	company := e.SeedCompany(t, "Brandt Automotive", nil)
	for _, mail := range []ids.UUID{restricted, ordinary} {
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`, mail, company)
	}
	registry := NewRegistry(e.Pool, SendPath{})
	agent := agentOfTheAdmin(t, e)
	invoke := func(tool, args string) (string, error) {
		out, err := registry.Invoke(agent, tool, json.RawMessage(args))
		return string(out), err
	}

	if out, err := invoke("read_record", fmt.Sprintf(`{"record_type":"activity","id":%q}`, ordinary)); err != nil || !strings.Contains(out, "please send the offer") {
		t.Fatalf("the ordinary mail must read in full (%v):\n%s", err, out)
	}
	if out, err := invoke("read_record", fmt.Sprintf(`{"record_type":"activity","id":%q}`, restricted)); err == nil && strings.Contains(out, "disputed address") {
		t.Errorf("read_record handed an agent the restricted contact's mail:\n%s", out)
	}
	out, err := invoke("list_records", fmt.Sprintf(`{"record_type":"activity","filters":{"entity_type":"company","entity_id":%q}}`, company))
	if err != nil {
		t.Fatalf("listing the company's mail: %v", err)
	}
	if !strings.Contains(out, "please send the offer") {
		t.Fatalf("the company's ordinary mail is missing from the list:\n%s", out)
	}
	if strings.Contains(out, "disputed address") {
		t.Errorf("list_records handed an agent the restricted contact's mail:\n%s", out)
	}
	if !errors.Is(func() error {
		_, err := invoke("read_record", fmt.Sprintf(`{"record_type":"contact","id":%q}`, anna))
		return err
	}(), apperrors.ErrNotFound) {
		t.Error("read_record answered an agent with the restricted contact")
	}
	if out, err := invoke("list_records", `{"record_type":"contact"}`); err != nil || strings.Contains(out, anna.String()) {
		t.Errorf("list_records named the restricted contact to an agent (%v):\n%s", err, out)
	}
}
