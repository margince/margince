// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

// Replying as a shared mailbox, on real Postgres. Three questions: who may
// choose the mailbox, who may still send through it at transmit time, and what
// the admin's switch records.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const replyScope = "Mail.Send"

// replyingMailbox is a Microsoft-like provider with no network that can send
// as a shared mailbox.
type replyingMailbox struct {
	deliveringMailbox
	opened []connector.Auth
}

var (
	_ connector.SharedMailboxSender = (*replyingMailbox)(nil)
	_ connector.EmailSender         = (*replyingMailbox)(nil)
)

func (m *replyingMailbox) ProbeShared(_ context.Context, auth connector.Auth) (connector.Auth, error) {
	return auth, nil
}

func (m *replyingMailbox) OpenShared(auth connector.Auth) (connector.Connector, connector.Auth, error) {
	return m, auth, nil
}

func (m *replyingMailbox) SharedSendScope() string { return replyScope }

func (m *replyingMailbox) OpenSharedSender(auth connector.Auth) (connector.EmailSender, connector.Auth, error) {
	m.opened = append(m.opened, auth)
	return m, connector.Auth("sender-state"), nil
}

func (m *replyingMailbox) SendEmail(context.Context, connector.Auth, connector.EmailMessage) (connector.SendReceipt, error) {
	return connector.SendReceipt{}, nil
}

// connectReplying connects sales@ over a provider that can send as it, read by team.
func (s *sharedMailboxSuite) connectReplying(t *testing.T, team ids.UUID) (ids.UUID, *replyingMailbox) {
	t.Helper()
	provider := &replyingMailbox{name: "graph", address: salesMailbox}
	s.registry.Register(provider)
	id, err := s.registry.ConnectShared(withReadScope(s.admin()), capture.SharedConnect{
		Provider: "graph", Address: salesMailbox, ReaderTeam: team, Auth: connector.Auth("app-credential"),
	})
	if err != nil {
		t.Fatalf("connecting the shared mailbox: %v", err)
	}
	return id, provider
}

func (s *sharedMailboxSuite) setReplies(t *testing.T, box ids.UUID, on bool) int {
	t.Helper()
	rec := s.call(s.admin(), t, http.MethodPatch, "/shared-mailboxes/"+box.String(), map[string]any{"reply_enabled": on, "confirm": false})
	return rec.Code
}

func (s *sharedMailboxSuite) chooseFor(t *testing.T, seat ids.UUID, team ids.UUID, from ids.UUID) (capture.SendingMailbox, error) {
	t.Helper()
	ctx := s.e.As(seat, []ids.UUID{team}, integration.RepPerms)
	return s.registry.ChooseSendingMailbox(ctx, ids.From[ids.UserKind](seat), from, ids.ActivityID{}, []string{"graph"},
		func(m capture.SendingMailbox) (bool, error) {
			return slices.Contains(m.ProviderScopes, replyScope), nil
		})
}

func TestOnlyALiveMemberOfTheReaderTeamChoosesTheMailboxOnceRepliesAreOn(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box, _ := s.connectReplying(t, e.Team1)
	if got := e.WsCount(t, `SELECT count(*) FROM team_membership WHERE team_id = $1 AND user_id = $2`, e.Team1, e.Rep2); got != 1 {
		t.Fatal("the fixture's Rep2 must read Team1")
	}

	if chosen, err := s.chooseFor(t, e.Rep2, e.Team1, box); err == nil && chosen.ID == box {
		t.Fatal("a reader chose the mailbox before an admin turned replies on")
	}
	if code := s.setReplies(t, box, true); code != http.StatusOK {
		t.Fatalf("turning replies on answered %d, want 200", code)
	}
	chosen, err := s.chooseFor(t, e.Rep2, e.Team1, box)
	if err != nil || chosen.ID != box || !chosen.Shared || chosen.Provider != "graph" {
		t.Fatalf("a reader choosing the mailbox got %+v, %v; want the shared mailbox", chosen, err)
	}

	if chosen, err := s.chooseFor(t, e.Rep3, e.Team2, box); err == nil && !chosen.ID.IsZero() {
		t.Fatalf("a member of another team chose %+v, want nothing", chosen)
	}
	if chosen, err := s.chooseFor(t, e.Rep2, e.Team1, ids.UUID{}); err == nil && chosen.ID == box {
		t.Fatal("a send that names no mailbox must never default to the shared one")
	}

	if got := e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
		WHERE a.entity_id = $1 AND o.envelope->>'payload' LIKE '%reply_changed%' AND a.after->>'reply_enabled' = 'true' AND a.before->>'reply_enabled' = 'false'`, box); got != 1 {
		t.Fatalf("turning replies on wrote %d audit rows with their event, want 1", got)
	}
}

func TestAnAdminCannotTurnRepliesOnForAMailboxThatOnlyCaptures(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.connect(t, s.e.Team1)
	rec := s.call(s.admin(), t, http.MethodPatch, "/shared-mailboxes/"+box.Id.String(), map[string]any{"reply_enabled": true, "confirm": false})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "shared_mailbox_cannot_send") {
		t.Fatalf("turning replies on for an IMAP mailbox answered %d %s, want 422 shared_mailbox_cannot_send", rec.Code, rec.Body)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND cardinality(coalesce(provider_scopes, '{}')) = 0`, box.Id); got != 1 {
		t.Fatal("a refused switch must leave the mailbox as it was")
	}
}

func TestTheCredentialOfASharedMailboxOpensOnlyForASeatThatStillReadsIt(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box, provider := s.connectReplying(t, e.Team1)
	if code := s.setReplies(t, box, true); code != http.StatusOK {
		t.Fatalf("turning replies on answered %d, want 200", code)
	}
	mailbox := capture.Mailbox{Provider: "graph", ID: box}
	reader := ids.From[ids.UserKind](e.Rep2)
	if _, _, granted, err := s.registry.SenderFor(s.admin(), reader, mailbox); err != nil || !slices.Contains(granted, replyScope) {
		t.Fatalf("a reader's send resolved %v, granted %v; want the sender with the send scope", err, granted)
	}
	if len(provider.opened) != 1 {
		t.Fatalf("the provider opened %d senders, want 1", len(provider.opened))
	}

	if _, _, _, err := s.registry.SenderFor(s.admin(), ids.From[ids.UserKind](e.Rep3), mailbox); !errors.Is(err, capture.ErrSharedReplyRefused) {
		t.Fatalf("a seat outside the team resolved %v, want ErrSharedReplyRefused", err)
	}

	e.WsExec(t, `DELETE FROM team_membership WHERE team_id = $1 AND user_id = $2`, e.Team1, e.Rep2)
	if _, _, _, err := s.registry.SenderFor(s.admin(), reader, mailbox); !errors.Is(err, capture.ErrSharedReplyRefused) {
		t.Fatalf("a seat that left the team resolved %v at transmit time, want ErrSharedReplyRefused", err)
	}
	e.WsExec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, e.Team1, e.Rep2)
	if code := s.setReplies(t, box, false); code != http.StatusOK {
		t.Fatalf("turning replies off answered %d, want 200", code)
	}
	if _, _, _, err := s.registry.SenderFor(s.admin(), reader, mailbox); !errors.Is(err, capture.ErrSharedReplyRefused) {
		t.Fatalf("a send after replies were turned off resolved %v, want ErrSharedReplyRefused", err)
	}
}

func (s *sharedMailboxSuite) replyableFor(t *testing.T, seat, team ids.UUID) []string {
	t.Helper()
	rec := s.call(s.e.As(seat, []ids.UUID{team}, integration.RepPerms), t, http.MethodGet, "/shared-mailboxes/replyable", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("the replyable list answered %d: %s", rec.Code, rec.Body)
	}
	var list crmcontracts.ReplyableSharedMailboxList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, item := range list.Items {
		out = append(out, item.Id.String()+" "+item.Address)
	}
	return out
}

func TestAMemberListsOnlyTheSharedMailboxesTheyMayReplyAs(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box, _ := s.connectReplying(t, e.Team1)
	want := []string{box.String() + " " + salesMailbox}

	if got := s.replyableFor(t, e.Rep2, e.Team1); len(got) != 0 {
		t.Fatalf("a reader sees %v before replies are on, want nothing", got)
	}
	if code := s.setReplies(t, box, true); code != http.StatusOK {
		t.Fatalf("turning replies on answered %d", code)
	}
	if got := s.replyableFor(t, e.Rep2, e.Team1); !slices.Equal(got, want) {
		t.Fatalf("a reader sees %v, want %v", got, want)
	}
	if got := s.replyableFor(t, e.Rep3, e.Team2); len(got) != 0 {
		t.Fatalf("a member of another team sees %v, want nothing", got)
	}
	if code := s.setReplies(t, box, false); code != http.StatusOK {
		t.Fatalf("turning replies off answered %d", code)
	}
	if got := s.replyableFor(t, e.Rep2, e.Team1); len(got) != 0 {
		t.Fatalf("a reader sees %v after replies are off, want nothing", got)
	}
	agent := e.AgentFor(t, e.Rep2, []ids.UUID{e.Team1}, integration.RepPerms)
	if rec := s.call(agent, t, http.MethodGet, "/shared-mailboxes/replyable", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("an agent listing the mailboxes answered %d, want 403", rec.Code)
	}
}
