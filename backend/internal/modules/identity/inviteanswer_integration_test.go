// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requestEnder ends a request at one statement of its transaction, the way a
// failed read or a dropped client ends it. With atCommit it waits for the
// COMMIT after the marked statement, so whatever the handler does after the
// write runs on an ended request.
type requestEnder struct {
	inner    pgx.QueryTracer
	marker   string
	atCommit bool
	end      context.CancelFunc

	mu     sync.Mutex
	marked bool
}

func (r *requestEnder) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	r.mu.Lock()
	switch {
	case strings.Contains(data.SQL, r.marker) && !r.atCommit:
		r.end()
	case strings.Contains(data.SQL, r.marker):
		r.marked = true
	}
	r.mu.Unlock()
	if r.inner == nil {
		return ctx
	}
	return r.inner.TraceQueryStart(ctx, conn, data)
}

func (r *requestEnder) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if r.inner != nil {
		r.inner.TraceQueryEnd(ctx, conn, data)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.atCommit && r.marked && data.CommandTag.String() == "COMMIT" {
		r.end()
	}
}

// endingRequests points the env's service at a pool whose statements pass
// through ender, and answers the request context ender ends.
func endingRequests(t *testing.T, e *revocationEnv, ender *requestEnder) context.Context {
	t.Helper()
	ctx, end := context.WithCancel(withIdentity(e.wsCtx(e.admin), e.admin))
	t.Cleanup(end)
	ender.end = end
	pool, err := testdb.OwnPoolWith(context.Background(), os.Getenv("MARGINCE_TEST_APP_DSN"), func(cfg *pgxpool.Config) {
		ender.inner = cfg.ConnConfig.Tracer
		cfg.ConnConfig.Tracer = ender
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	e.svc.db = database.BindTo(pool, e.ws)
	return ctx
}

func postInvite(ctx context.Context, h Handlers, email string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	body := `{"email":"` + email + `","display_name":"New Member","role":"rep"}`
	h.InviteUser(rec, httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader(body)).WithContext(ctx))
	return rec
}

func membersAt(t *testing.T, e *revocationEnv, email string) []ids.UUID {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), `SELECT id FROM app_user WHERE email = $1`, email)
	if err != nil {
		t.Fatal(err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func answeredMember(t *testing.T, rec *httptest.ResponseRecorder) crmcontracts.User {
	t.Helper()
	var member crmcontracts.User
	if err := json.Unmarshal(rec.Body.Bytes(), &member); err != nil {
		t.Fatalf("the answer is not a member: %v: %s", err, rec.Body)
	}
	return member
}

// A request that ends once the invite has committed still answers the member
// it created, because nothing after the commit can fail.
func TestAnInviteThatCommittedAnswersTheMember(t *testing.T) {
	e := setupRevocationEnv(t, "invite-answer-commit")
	ctx := endingRequests(t, e, &requestEnder{marker: "INSERT INTO app_user", atCommit: true})
	h := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")

	rec := postInvite(ctx, h, "committed@acme.test")
	if rec.Code != http.StatusCreated {
		t.Fatalf("an invite whose member was committed answered %d: %s", rec.Code, rec.Body)
	}
	member := answeredMember(t, rec)
	created := membersAt(t, e, "committed@acme.test")
	if len(created) != 1 || created[0] != ids.UUID(member.Id) {
		t.Fatalf("the answer names member %s, the roster holds %v", member.Id, created)
	}
	if member.Status != crmcontracts.UserStatusInvited {
		t.Errorf("the answered member is %q, want invited", member.Status)
	}
}

// A read of the answer that fails inside the invite rolls the member back. So
// the refusal is true, and the invite sent again creates one member.
func TestAnInviteWhoseAnswerCannotBeReadCreatesNobody(t *testing.T) {
	e := setupRevocationEnv(t, "invite-answer-read")
	ctx := endingRequests(t, e, &requestEnder{marker: getUserQuery})
	h := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")

	if rec := postInvite(ctx, h, "unread@acme.test"); rec.Code == http.StatusCreated {
		t.Fatalf("an invite whose answer could not be read answered 201: %s", rec.Body)
	}
	if created := membersAt(t, e, "unread@acme.test"); len(created) != 0 {
		t.Fatalf("a refused invite left members %v behind", created)
	}

	retry := postInvite(withIdentity(e.wsCtx(e.admin), e.admin), h, "unread@acme.test")
	if retry.Code != http.StatusCreated {
		t.Fatalf("the invite sent again answered %d: %s", retry.Code, retry.Body)
	}
	if created := membersAt(t, e, "unread@acme.test"); len(created) != 1 {
		t.Fatalf("the invite sent again left %d members, want 1", len(created))
	}
}

// An invite sent again for a member who is waiting on one names that member's
// state, and creates nobody.
func TestAnInviteSentAgainNamesTheWaitingMember(t *testing.T) {
	e := setupRevocationEnv(t, "invite-answer-again")
	h := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")
	ctx := withIdentity(e.wsCtx(e.admin), e.admin)
	if rec := postInvite(ctx, h, "twice@acme.test"); rec.Code != http.StatusCreated {
		t.Fatalf("the first invite answered %d: %s", rec.Code, rec.Body)
	}

	again := postInvite(ctx, h, "Twice@acme.test")
	if again.Code != http.StatusConflict || !strings.Contains(again.Body.String(), `"already_invited"`) {
		t.Fatalf("the invite sent again answered %d %s, want 409 already_invited", again.Code, again.Body)
	}
	if created := membersAt(t, e, "twice@acme.test"); len(created) != 1 {
		t.Fatalf("the invite sent again left %d members, want 1", len(created))
	}
}

// A resend whose request ends once the new link has committed still answers
// the member, as the invite does.
func TestAResendThatCommittedAnswersTheMember(t *testing.T) {
	e := setupRevocationEnv(t, "invite-answer-resend")
	unmailed := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")
	id := inviteThrough(t, e, unmailed, "resent@acme.test")
	h, sent := mailingHandlers(e, &capturedMail{})
	ctx := endingRequests(t, e, &requestEnder{marker: "INSERT INTO auth_token", atCommit: true})

	rec := httptest.NewRecorder()
	h.ResendUserInvite(rec, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx), id)
	<-sent
	if rec.Code != http.StatusOK {
		t.Fatalf("a resend whose link was committed answered %d: %s", rec.Code, rec.Body)
	}
	if member := answeredMember(t, rec); member.Id != id {
		t.Errorf("the resend answered member %s, want %s", member.Id, id)
	}
}

// With every seat taken, an invite sent again for a waiting invitee names that
// invitee only to a caller who may read the roster. A caller who may only
// create members gets the seat refusal, as for any other address.
func TestOnlyARosterReaderLearnsAnAddressWaitsOnAnInvitation(t *testing.T) {
	e := setupRevocationEnv(t, "invite-answer-reader")
	again := InviteUserInput{Email: "waiting@" + e.slug + ".test", DisplayName: "Waiting", Role: "rep"}
	if _, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, again); err != nil {
		t.Fatal(err)
	}
	// No stored role holds create without read, but a passport narrowing a
	// grant (Permissions.Within) can leave that.
	creator := e.admin
	creator.Permissions.Objects = maps.Clone(e.admin.Permissions.Objects)
	creator.Permissions.Objects[objectUserAdmin] = principal.ObjectGrant{Create: true}
	licenseFor(t, e, licensedSeats(seatsInUse(t, e)))

	if _, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, again); !errors.Is(err, errAlreadyInvited) {
		t.Errorf("a roster reader inviting the waiting address again: %v, want errAlreadyInvited", err)
	}
	if _, _, err := e.svc.InviteUser(e.wsCtx(creator), creator, again); !errors.Is(err, apperrors.ErrSeatLimitReached) {
		t.Errorf("a create-only caller inviting the waiting address again: %v, want the seat refusal", err)
	}
}
