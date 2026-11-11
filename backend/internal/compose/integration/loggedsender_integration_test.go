// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A request logged as a received message asks for a follow-up from its author
// alone, not from everyone on the conversation. The
// fixtures use the create door that POST /v1/activities and log_activity share.
// The check is the send gate's requested_followup preview.

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/provensenders"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	alexAddress = "alex.moreau@lumenfield.example"
	beaAddress  = "bea.lindqvist@lumenfield.example"
)

// senderEnv is a company with two contacts who talk to us in one group chat.
type senderEnv struct {
	e         *Env
	gate      *consent.Gate
	company   ids.UUID
	alex, bea ids.UUID
}

func setupSenderEnv(t *testing.T) senderEnv {
	t.Helper()
	e := Setup(t)
	return senderEnv{
		e:       e,
		gate:    consent.NewGate(consent.NewStore(compose.InstallationDB(e.Pool))),
		company: e.SeedCompany(t, "Lumenfield Optics", &e.AdminUser),
		alex:    seedRecipient(t, e, "Alex Moreau", alexAddress, nil),
		bea:     seedRecipient(t, e, "Bea Lindqvist", beaAddress, nil),
	}
}

// logInbound logs a received activity the way a REST caller or the agent's
// log_activity call sends it.
func (s senderEnv) logInbound(t *testing.T, fields map[string]any, links ...ids.UUID) (ids.UUID, error) {
	t.Helper()
	wire := make([]map[string]any, 0, len(links))
	for _, link := range links {
		kind := "contact"
		if link == s.company {
			kind = "company"
		}
		wire = append(wire, map[string]any{"entity_type": kind, "entity_id": link.String()})
	}
	body := map[string]any{
		"kind": "message", "channel_provider": "whatsapp", "direction": "inbound", "source": "manual",
		"body": "Could you send us the pricing for the lens line next week?", "links": wire,
	}
	maps.Copy(body, fields)
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var req crmcontracts.CreateActivityRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("decoding the create request: %v", err)
	}
	in, err := activities.LogActivityInputFrom(req)
	if err != nil {
		return ids.Nil, err
	}
	logged, _, err := s.e.Activities.LogActivity(s.e.Admin(), in)
	return ids.UUID(logged.Id), err
}

// followupAllowed asks the send gate whether a follow-up the address asked
// for may go out.
func (s senderEnv) followupAllowed(t *testing.T, address string) bool {
	t.Helper()
	set, err := s.gate.Preview(s.e.Admin(), commsauthz.Request{
		Recipients: []connector.Recipient{{Email: address}}, Context: commsauthz.CategoryRequestedFollowup,
	})
	if err != nil {
		t.Fatalf("asking the gate about %s: %v", address, err)
	}
	if len(set.Decisions) != 1 {
		t.Fatalf("asked about one recipient, got %d decisions", len(set.Decisions))
	}
	return set.Decisions[0].Verdict == commsauthz.VerdictAllow
}

func (s senderEnv) roleOf(t *testing.T, activity, contact ids.UUID) string {
	t.Helper()
	return s.e.WsScalar(t, `SELECT coalesce(string_agg(role, ',' ORDER BY role), '') FROM activity_participant
		WHERE activity_id = $1 AND contact_id = $2`, activity, contact)
}

func TestAGroupChatWithNoNamedSenderIsNobodysRequest(t *testing.T) {
	s := setupSenderEnv(t)
	chat, err := s.logInbound(t, nil, s.alex, s.bea, s.company)
	if err != nil {
		t.Fatalf("logging the group chat: %v", err)
	}
	for name, contact := range map[string]ids.UUID{"Alex": s.alex, "Bea": s.bea} {
		if role := s.roleOf(t, chat, contact); role != "cc" {
			t.Errorf("%s is stored as %q on a group chat with no named sender, want cc", name, role)
		}
	}
	for _, address := range []string{alexAddress, beaAddress} {
		if s.followupAllowed(t, address) {
			t.Errorf("a follow-up to %s rests on a group chat nobody was named as writing", address)
		}
	}
}

func TestTheNamedSenderAloneAskedForTheFollowUp(t *testing.T) {
	s := setupSenderEnv(t)
	chat, err := s.logInbound(t, map[string]any{"sender_contact_id": s.alex.String()}, s.alex, s.bea)
	if err != nil {
		t.Fatalf("logging the group chat: %v", err)
	}
	if role := s.roleOf(t, chat, s.alex); role != "from" {
		t.Errorf("the named sender is stored as %q, want from", role)
	}
	if !s.followupAllowed(t, alexAddress) {
		t.Error("a follow-up to the contact who wrote the request was refused")
	}
	if s.followupAllowed(t, beaAddress) {
		t.Error("a follow-up to the other member of the group chat was allowed on Alex's request")
	}
}

// A chat linked to one contact keeps naming that contact as its sender.
func TestASingleLinkedContactStillWroteTheMessage(t *testing.T) {
	s := setupSenderEnv(t)
	if _, err := s.logInbound(t, nil, s.alex); err != nil {
		t.Fatalf("logging the chat: %v", err)
	}
	if !s.followupAllowed(t, alexAddress) {
		t.Error("a follow-up to the only contact on the chat was refused")
	}
}

func TestANamedSenderMustBeLinked(t *testing.T) {
	s := setupSenderEnv(t)
	cara := seedRecipient(t, s.e, "Cara Okafor", "cara.okafor@lumenfield.example", nil)
	cases := map[string]struct {
		fields map[string]any
		code   string
	}{
		"a contact not in links": {map[string]any{"sender_contact_id": cara.String()}, "sender_not_linked"},
		"an outbound message":    {map[string]any{"sender_contact_id": s.alex.String(), "direction": "outbound"}, "field_not_valid_for_kind"},
	}
	for name, c := range cases {
		c.fields["body"] = "Refused: " + name
		_, err := s.logInbound(t, c.fields, s.alex, s.bea)
		var fault interface {
			FieldFault() (field, code, message string)
		}
		if !errors.As(err, &fault) {
			t.Fatalf("%s: err = %v, want a field refusal", name, err)
		}
		if field, code, _ := fault.FieldFault(); field != "sender_contact_id" || code != c.code {
			t.Errorf("%s: refused %s with %s, want sender_contact_id with %s", name, field, code, c.code)
		}
		if n := s.e.WsCount(t, `SELECT count(*) FROM activity WHERE body = $1`, "Refused: "+name); n != 0 {
			t.Errorf("%s: the refused activity was written anyway", name)
		}
	}
}

// A mail logged under the company names its sender only by address. Linking a
// contact later makes them the sender only when that address is theirs.
func TestALaterLinkNamesOnlyTheProvenSender(t *testing.T) {
	s := setupSenderEnv(t)
	mail, err := s.logInbound(t, map[string]any{
		"kind": "email", "channel_provider": nil, "subject": "Lens pricing",
		"participants": map[string]any{"from": alexAddress, "to": []string{}},
	}, s.company)
	if err != nil {
		t.Fatalf("logging the mail under the company: %v", err)
	}
	relink := func(contact ids.UUID) {
		if _, err := s.e.Activities.RelinkActivity(s.e.Admin(), ids.From[ids.ActivityKind](mail),
			activities.RelinkActivityInput{EntityType: "contact", EntityID: contact}); err != nil {
			t.Fatalf("linking the mail to a contact: %v", err)
		}
	}

	relink(s.bea)
	if role := s.roleOf(t, mail, s.bea); role != "" {
		t.Errorf("a contact the stated address does not name was stored as %q", role)
	}
	if s.followupAllowed(t, beaAddress) {
		t.Error("a follow-up to a contact linked to Alex's mail was allowed")
	}

	relink(s.alex)
	if role := s.roleOf(t, mail, s.alex); role != "from" {
		t.Errorf("the contact whose address sent the mail is stored as %q after the link, want from", role)
	}
	if !s.followupAllowed(t, alexAddress) {
		t.Error("a follow-up to the proven sender was refused")
	}
	if n := s.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'activity_relink'
		AND after ->> 'proven_sender' = $2`, mail, s.alex.String()); n != 1 {
		t.Errorf("%d relink audit rows name Alex as the proven sender, want 1", n)
	}
}

// Replacing a message's contact never carries its author to the new contact.
// The new contact becomes the sender only when the stated address is theirs.
func TestAReplacingLinkKeepsTheAuthorOnProof(t *testing.T) {
	s := setupSenderEnv(t)
	logged := func(fields map[string]any) func() ids.UUID {
		return func() ids.UUID {
			id, err := s.logInbound(t, fields, s.alex)
			if err != nil {
				t.Fatalf("logging the message under Alex: %v", err)
			}
			return id
		}
	}
	cases := map[string]struct {
		log func() ids.UUID
		// statesAddress: the message keeps Alex's address as its bare sender, so
		// moving it back to Alex proves him again.
		statesAddress bool
	}{
		"a mail logged from Alex's address": {logged(map[string]any{
			"kind": "email", "channel_provider": nil, "subject": "Lens pricing",
			"participants": map[string]any{"from": alexAddress, "to": []string{}},
		}), true},
		// The participant backfill names Alex and his address on one row.
		"an older mail from Alex's address": {func() ids.UUID {
			id := s.olderInbound(t, alexAddress, s.alex)
			if _, err := s.e.Activities.BackfillParticipantsBatch(s.e.Admin(), 50); err != nil {
				t.Fatalf("backfilling the older mail's participants: %v", err)
			}
			return id
		}, true},
		"a chat naming Alex alone": {logged(map[string]any{"body": "Could you send the lens line pricing to Alex?"}), false},
	}
	for name, c := range cases {
		message := c.log()
		if role := s.roleOf(t, message, s.alex); role != "from" {
			t.Fatalf("%s: Alex is stored as %q before the change, want from", name, role)
		}
		replace := func(contact ids.UUID) {
			if _, err := s.e.Activities.RelinkActivity(s.e.Admin(), ids.From[ids.ActivityKind](message),
				activities.RelinkActivityInput{EntityType: "contact", EntityID: contact, ReplaceExistingOfType: true}); err != nil {
				t.Fatalf("%s: moving it to another contact: %v", name, err)
			}
		}
		replace(s.bea)
		if role := s.roleOf(t, message, s.bea); role == "from" {
			t.Errorf("%s: moving it to Bea made her the author of Alex's message", name)
		}
		if s.followupAllowed(t, beaAddress) {
			t.Errorf("%s: a follow-up to Bea rests on Alex's message", name)
		}
		if !c.statesAddress {
			continue
		}
		if n := s.e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1
			AND role = 'from' AND contact_id IS NULL AND address = $2`, message, alexAddress); n != 1 {
			t.Errorf("%s: %d bare sender rows state Alex's address after the move, want 1", name, n)
		}
		replace(s.alex)
		if role := s.roleOf(t, message, s.alex); role != "from" {
			t.Errorf("%s: moved back to Alex, whose address sent it, he is stored as %q, want from", name, role)
		}
	}
}

// olderInbound is a received mail as captures before sender rows stored it: a
// contact link and a counterparty address, and no participant at all.
func (s senderEnv) olderInbound(t *testing.T, counterparty string, links ...ids.UUID) ids.UUID {
	t.Helper()
	id, err := s.logInbound(t, map[string]any{"kind": "email", "channel_provider": nil, "subject": "Older mail"}, links...)
	if err != nil {
		t.Fatalf("logging the older mail: %v", err)
	}
	s.e.WsExec(t, `DELETE FROM activity_participant WHERE activity_id = $1`, id)
	s.e.WsExec(t, `UPDATE activity SET counterparty_email = $2 WHERE id = $1`, id, counterparty)
	return id
}

func TestTheSenderRepairStampsOnlyAProvenSender(t *testing.T) {
	s := setupSenderEnv(t)
	ctx := principal.SystemActing(s.e.Admin(), "system:datatask-test")
	proven := s.olderInbound(t, alexAddress, s.alex)
	// A group member the address does not name.
	unnamed := s.olderInbound(t, "someone.else@lumenfield.example", s.alex, s.bea)
	// The address moved from Dana to Eve, so it proves nothing about who wrote
	// the older mail.
	dana := seedRecipient(t, s.e, "Dana Whitfield", "orders@lumenfield.example", nil)
	if _, err := s.e.Contacts.UpdateContact(s.e.Admin(), ids.From[ids.ContactKind](dana),
		contacts.UpdateContactInput{Emails: []contacts.ContactEmailInput{}, Source: "manual"}); err != nil {
		t.Fatalf("taking the address off Dana: %v", err)
	}
	eve := seedRecipient(t, s.e, "Eve Marsh", "orders@lumenfield.example", nil)
	reassigned := s.olderInbound(t, "orders@lumenfield.example", eve)

	if s.followupAllowed(t, alexAddress) {
		t.Fatal("the older mail already counts before the repair, so the repair proves nothing")
	}
	if got := provenSendersPreview(ctx, t, s.e); got != 1 {
		t.Errorf("the repair's preview counts %d messages, want 1", got)
	}
	s.e.RunDataTask(ctx, t, provensenders.DataTask())

	if role := s.roleOf(t, proven, s.alex); role != "from" {
		t.Errorf("the proven sender is stored as %q after the repair, want from", role)
	}
	if !s.followupAllowed(t, alexAddress) {
		t.Error("a follow-up to the proven sender is still refused after the repair")
	}
	for name, c := range map[string]struct{ activity, contact ids.UUID }{
		"Alex on the group mail": {unnamed, s.alex}, "Bea on the group mail": {unnamed, s.bea},
		"Eve on the reassigned address": {reassigned, eve},
	} {
		if role := s.roleOf(t, c.activity, c.contact); role != "" {
			t.Errorf("the repair stamped %s as %q", name, role)
		}
	}
	if got := provenSendersPreview(ctx, t, s.e); got != 0 {
		t.Errorf("the repair's preview still counts %d messages", got)
	}
	if n := s.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after ->> 'proven_sender' = $2`,
		proven, s.alex.String()); n != 1 {
		t.Errorf("%d audit rows name the repaired sender, want 1", n)
	}
}

func provenSendersPreview(ctx context.Context, t *testing.T, e *Env) int64 {
	t.Helper()
	count, _, err := provensenders.DataTask().Steps[0].Estimate(ctx, e.Pool, 100)
	if err != nil {
		t.Fatalf("previewing the repair: %v", err)
	}
	return count
}

// A sender row that names Ada and carries her address stays Ada's after the
// address moves to Bo. A draft written to Bo reads none of that mail. The
// participant backfill writes this row shape for older mail.
func TestMailFromAReassignedAddressStaysWithItsAuthor(t *testing.T) {
	s := setupSenderEnv(t)
	shared := "desk@lumenfield.example"
	ada := seedRecipient(t, s.e, "Ada Brennan", shared, nil)
	mail := s.olderInbound(t, shared, ada)
	if _, err := s.e.Activities.BackfillParticipantsBatch(s.e.Admin(), 50); err != nil {
		t.Fatalf("backfilling the older mail's participants: %v", err)
	}
	if n := s.e.WsCount(t, `SELECT count(*) FROM activity_participant
		WHERE activity_id = $1 AND role = 'from' AND contact_id = $2 AND address = $3`, mail, ada, shared); n != 1 {
		t.Fatalf("the backfill stored %d sender rows naming Ada with the address, want 1", n)
	}
	if _, err := s.e.Contacts.UpdateContact(s.e.Admin(), ids.From[ids.ContactKind](ada),
		contacts.UpdateContactInput{Emails: []contacts.ContactEmailInput{}, Source: "manual"}); err != nil {
		t.Fatalf("taking the address off Ada: %v", err)
	}
	bo := seedRecipient(t, s.e, "Bo Achterberg", shared, nil)

	partyTo := func(contact ids.UUID) bool {
		id := ids.From[ids.ContactKind](contact)
		var found bool
		if err := s.e.DB().Tx(s.e.Admin(), func(tx pgx.Tx) error {
			rows, _, err := s.e.Activities.ListActivitiesIn(s.e.Admin(), tx, activities.ListActivitiesInput{PartyContactID: &id, Limit: new(50)})
			for _, row := range rows {
				found = found || ids.UUID(row.Id) == mail
			}
			return err
		}); err != nil {
			t.Fatalf("reading the mail a draft to %s may use: %v", contact, err)
		}
		return found
	}
	if partyTo(bo) {
		t.Error("a draft to Bo reads mail Ada wrote from the address before it was his")
	}
	if !partyTo(ada) {
		t.Error("Ada is no longer a party to the mail she wrote")
	}
}
