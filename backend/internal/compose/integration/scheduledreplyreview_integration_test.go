// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestReviewingAReplyReleasesTheMessageButANewerReplyPausesItAgain(t *testing.T) {
	for _, newer := range []bool{false, true} {
		t.Run(map[bool]string{false: "reviewed", true: "new reply"}[newer], func(t *testing.T) {
			p := setupPreflight(t)
			p.connect(t, gmailReadonlyScope, gmailSendScope)
			p.stakeADeal(t)
			id := p.scheduleClaiming(t, AnyMap{"communication_context": "active_deal_followup"})
			arrived := p.scheduledCreation(t, id).Add(time.Microsecond)
			reply := p.theyWrite(t, inbound{dated: arrived, arrived: arrived, authenticated: true})
			if reply.IsZero() {
				t.Fatal("the human reply was dropped before storage")
			}
			p.makeDue(t, id)
			p.fire(t, id)
			card, found := p.heldCardFor(t, id)
			if !found {
				status, reason := p.scheduledStatus(t, id)
				t.Fatalf("new reply raised no review card: status %q, reason %q", status, reason)
			}
			if status := p.Call(t, "POST", "/v1/approvals/"+card.ID+"/approve", AnyMap{}, nil, nil); status != http.StatusOK {
				t.Fatalf("reviewing the reply answered %d", status)
			}
			if newer {
				arrived = arrived.Add(time.Hour)
				p.theyWrite(t, inbound{dated: arrived, arrived: arrived, authenticated: true})
			}
			p.makeDue(t, id)
			p.fire(t, id)
			status, reason := p.scheduledStatus(t, id)
			if newer {
				if status != activities.ScheduledStatusHeld || reason != activities.HeldRecipientReplied {
					t.Fatalf("newer reply: status %q, reason %q", status, reason)
				}
			} else if !p.sent(t, id) {
				t.Fatalf("reviewed reply stopped the message again: %q, %q", status, reason)
			}
		})
	}
}

func TestAPrivateReplyToAColleagueDoesNotPauseTheSendersMessage(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	p.stakeADeal(t)
	id := p.scheduleClaiming(t, AnyMap{"communication_context": "active_deal_followup"})
	colleague := *p
	colleague.user = inviteReplyReader(t, p)
	colleague.connect(t, gmailReadonlyScope)
	arrived := p.scheduledCreation(t, id).Add(time.Microsecond)
	reply := p.theyWrite(t, inbound{
		dated: arrived, arrived: arrived, authenticated: true,
		owner: colleague.user, mailbox: "colleague@fable.test",
	})
	var thread string
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(colleague.privacyAdmin(t), `SELECT thread_key FROM activity WHERE id = $1`, reply).Scan(&thread)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := compose.NewThreadAudienceSetter(p.Pool).Decide(colleague.privacyAdmin(t), thread, false); err != nil {
		t.Fatal(err)
	}
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return auth.EnsureActivityContentVisibleLive(p.privacyAdmin(t), tx, reply)
	}); !errors.Is(err, apperrors.ErrPermissionDenied) && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("fixture reply should be unreadable to the sender: %v", err)
	}
	p.makeDue(t, id)
	p.fire(t, id)
	if !p.sent(t, id) {
		status, reason := p.scheduledStatus(t, id)
		t.Fatalf("private colleague mail paused the sender's message: %q, %q", status, reason)
	}
}

func inviteReplyReader(t *testing.T, p *preflightEnv) string {
	t.Helper()
	p.DescribeCompany(t)
	var user struct {
		ID string `json:"id"`
	}
	if status := p.Call(t, "POST", "/v1/users", AnyMap{"email": "colleague@fable.test", "display_name": "Colleague", "role": "rep"}, nil, &user); status != http.StatusCreated {
		t.Fatalf("inviting colleague answered %d", status)
	}
	var link struct {
		URL string `json:"set_password_url"`
	}
	if status := p.Call(t, "POST", "/v1/users/"+user.ID+"/password-link", nil, nil, &link); status != http.StatusCreated {
		t.Fatalf("issuing password link answered %d", status)
	}
	_, query, found := strings.Cut(link.URL, "?token=")
	if !found {
		t.Fatal("password link contains no token")
	}
	token, err := url.QueryUnescape(query)
	if err != nil {
		t.Fatal(err)
	}
	if status := p.Call(t, "POST", "/v1/auth/reset-password", AnyMap{"token": token, "new_password": "colleague-reply-test-password"}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("activating colleague answered %d", status)
	}
	return user.ID
}
