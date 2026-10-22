// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The system reads over real Postgres, reached the way a unit reaches them:
// from a subscription delivery through the production consumer.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assignments"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// dealReaderGrants opens every deal to its holder.
const dealReaderGrants = `{"objects":{"deal":{"read":true}},"row_scope":"all"}`

// dealCardFields are the fields a unit mirroring a deal shows.
var dealCardFields = []string{"name", "status", "stage_id", "amount_minor", "currency", "expected_close_date", "owner_id"}

// systemReadEnv is one deal owned by AdminUser, with Rep1 and Rep2 assigned.
// AdminUser, Rep1 and Rep3 may read every deal; Rep2 holds no grant at all.
type systemReadEnv struct {
	*integration.Env
	deal ids.UUID
	// roles are the record roles a user and a team may hold on the deal.
	roles map[crmcontracts.AssignmentSubjectKind]ids.UUID
}

func setupSystemRead(t *testing.T) *systemReadEnv {
	t.Helper()
	e := integration.Setup(t)
	bindRuntimeForTest(t, e.Pool, nil)
	composeReader(t, extension.SystemReadDeals, extension.SystemReadAuthority, extension.SystemReadDirectory)
	s := &systemReadEnv{Env: e}
	for _, seat := range []ids.UUID{e.AdminUser, e.Rep1, e.Rep3} {
		s.grant(t, seat, dealReaderGrants)
	}
	ctx := e.Admin()
	pipeline, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Sales"})
	if err != nil {
		t.Fatal(err)
	}
	pipelineID := ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
	stage, err := e.Deals.CreateStage(ctx, deals.CreateStageInput{PipelineID: pipelineID, Name: "Qualified", Position: 1, Semantic: "open"})
	if err != nil {
		t.Fatal(err)
	}
	owner := ids.From[ids.UserKind](e.AdminUser)
	amount, currency := int64(1200000), "EUR"
	created, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{
		Name: "Fleet renewal", PipelineID: pipelineID, StageID: ids.From[ids.StageKind](ids.UUID(stage.Id)),
		OwnerID: &owner, Source: "manual", AmountMinor: &amount, Currency: &currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.deal = ids.UUID(created.Id)
	s.roles = map[crmcontracts.AssignmentSubjectKind]ids.UUID{}
	for kind, key := range map[crmcontracts.AssignmentSubjectKind]string{
		crmcontracts.AssignmentSubjectKindUser: "sales_engineer", crmcontracts.AssignmentSubjectKindTeam: "technical_contact",
	} {
		var role ids.UUID
		s.read(t, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT id FROM record_role WHERE key = $1`, key).Scan(&role)
		})
		s.roles[kind] = role
	}
	s.assign(t, crmcontracts.AssignmentSubjectKindUser, e.Rep1)
	s.assign(t, crmcontracts.AssignmentSubjectKindUser, e.Rep2)
	return s
}

// grant gives seat a role of its own carrying permissions.
func (s *systemReadEnv) grant(t *testing.T, seat ids.UUID, permissions string) {
	t.Helper()
	key := "deal-reader-" + seat.String()
	s.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ($1, 'Deal reader', $2::jsonb)`, key, permissions)
	s.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT r.id, $1 FROM role r WHERE r.key = $2`, seat, key)
}

// assign puts a user or a team on the deal through the assignment store.
func (s *systemReadEnv) assign(t *testing.T, kind crmcontracts.AssignmentSubjectKind, subject ids.UUID) {
	t.Helper()
	if _, err := assignments.NewStore(s.DB()).CreateAssignment(s.Admin(), assignments.CreateAssignmentInput{
		RecordType: crmcontracts.AssignmentRecordTypeDeal, RecordID: s.deal,
		SubjectKind: kind, SubjectID: subject, RoleID: s.roles[kind],
	}); err != nil {
		t.Fatalf("assigning %s to the deal: %v", subject, err)
	}
}

// member adds an app_user to team and answers its id.
func (s *systemReadEnv) member(t *testing.T, team ids.UUID, email, status string, agent bool) ids.UUID {
	t.Helper()
	user := s.seat(t, email, status, agent)
	s.WsExec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, team, user)
	return user
}

// seat adds an app_user on no team and answers its id.
func (s *systemReadEnv) seat(t *testing.T, email, status string, agent bool) ids.UUID {
	t.Helper()
	user := ids.NewV7()
	s.WsExec(t, `INSERT INTO app_user (id, email, display_name, status, is_agent) VALUES ($1, $2, 'Desk', $3, $4)`,
		user, email, status, agent)
	return user
}

func (s *systemReadEnv) read(t *testing.T, fn func(context.Context, pgx.Tx) error) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), s.WS)
	if err := database.WithWorkspaceTx(ctx, s.Pool, func(tx pgx.Tx) error { return fn(ctx, tx) }); err != nil {
		t.Fatalf("reading the fixture's tables: %v", err)
	}
}

// onDelivery runs fn as a unit's handler for one deal.updated delivery,
// through the consumer the worker runs, and fails the test with what fn
// returned.
func (s *systemReadEnv) onDelivery(t *testing.T, fn func(context.Context, extension.Runtime) error) {
	t.Helper()
	sub := ComposedSubscription{Unit: "dealmirror", Version: "1.0.0", Sub: extension.Subscription{
		Name: "mirror", Events: []string{string(crmcontracts.DealUpdated)},
		Handle: func(ctx context.Context, rt extension.Runtime, d extension.Delivery) error {
			if rt.Caller() != (extension.Caller{}) {
				return fmt.Errorf("the delivery runs as %+v, want nobody", rt.Caller())
			}
			return fn(ctx, rt)
		},
	}}
	err := sub.Handler(s.Pool, quietTestLogger())(context.Background(), kevents.Envelope{
		EventID: ids.NewV7(), Type: string(crmcontracts.DealUpdated),
		Entity: kevents.EntityRef{Type: "deal", ID: s.deal},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (s *systemReadEnv) canRead(t *testing.T, user ids.UUID, fields ...string) bool {
	t.Helper()
	var shown bool
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		shown, err = rt.Authority().CanRead(ctx, extension.UserID(user.String()),
			extension.EntityRef{Type: "deal", ID: s.deal.String()}, fields)
		return err
	})
	return shown
}

func (s *systemReadEnv) responsible(t *testing.T) []extension.Responsible {
	t.Helper()
	var got []extension.Responsible
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		got, err = rt.Deals().Responsible(ctx, s.deal.String())
		return err
	})
	return got
}

func userIDs(rs []extension.Responsible) []extension.UserID {
	out := make([]extension.UserID, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.UserID)
	}
	return out
}

func asUser(id ids.UUID) extension.UserID { return extension.UserID(id.String()) }

func TestADeliveryReadsTheDealItHeardAbout(t *testing.T) {
	s := setupSystemRead(t)
	closeOn := time.Date(2031, time.March, 14, 0, 0, 0, 0, time.UTC)
	s.WsExec(t, `UPDATE deal SET expected_close_date = $2 WHERE id = $1`, s.deal, closeOn)
	var got extension.DealSummary
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		got, err = rt.Deals().Get(ctx, s.deal.String())
		return err
	})
	if got.ID != s.deal.String() || got.Name != "Fleet renewal" || got.StageName != "Qualified" || got.Status != "open" {
		t.Errorf("deal = %+v, want Fleet renewal, open in Qualified", got)
	}
	if got.AmountMinor == nil || *got.AmountMinor != 1200000 || got.Currency == nil || *got.Currency != "EUR" ||
		got.AmountDisplay != "12,000.00 EUR" {
		t.Errorf("amount = %v %v %q, want 1200000 EUR shown as 12,000.00 EUR", got.AmountMinor, got.Currency, got.AmountDisplay)
	}
	if got.ExpectedClose == nil || !got.ExpectedClose.Equal(closeOn) {
		t.Errorf("expected close = %v, want %v", got.ExpectedClose, closeOn)
	}
	if got.OwnerID != asUser(s.AdminUser) || got.Archived {
		t.Errorf("owner %q archived %v, want %s and live", got.OwnerID, got.Archived, s.AdminUser)
	}

	if _, err := s.Deals.ArchiveDeal(s.Admin(), ids.From[ids.DealKind](s.deal), nil); err != nil {
		t.Fatal(err)
	}
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		got, err = rt.Deals().Get(ctx, s.deal.String())
		return err
	})
	if !got.Archived {
		t.Error("an archived deal reads as live")
	}
}

// A currency with no minor unit prints no decimals, which a unit could not
// know without the core's table of currencies.
func TestAZeroDecimalAmountIsShownWithoutDecimals(t *testing.T) {
	s := setupSystemRead(t)
	for currency, want := range map[string]string{"JPY": "1,200,000 JPY", "VND": "1,200,000 VND"} {
		s.WsExec(t, `UPDATE deal SET currency = $2 WHERE id = $1`, s.deal, currency)
		s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
			got, err := rt.Deals().Get(ctx, s.deal.String())
			if err == nil && got.AmountDisplay != want {
				t.Errorf("%s: shown as %q, want %q", currency, got.AmountDisplay, want)
			}
			return err
		})
	}
	s.WsExec(t, `UPDATE deal SET amount_minor = NULL, currency = NULL WHERE id = $1`, s.deal)
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		got, err := rt.Deals().Get(ctx, s.deal.String())
		if err == nil && got.AmountDisplay != "" {
			t.Errorf("a deal with no amount is shown as %q", got.AmountDisplay)
		}
		return err
	})
}

func TestADealThatIsGoneReadsAsNotFound(t *testing.T) {
	s := setupSystemRead(t)
	erased := s.deal
	s.WsExec(t, `DELETE FROM deal WHERE id = $1`, erased)
	for name, id := range map[string]string{"never existed": ids.NewV7().String(), "erased": erased.String()} {
		s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
			if _, err := rt.Deals().Get(ctx, id); !errors.Is(err, extension.ErrNotFound) {
				t.Errorf("%s: Get err = %v, want ErrNotFound", name, err)
			}
			if _, err := rt.Deals().Responsible(ctx, id); !errors.Is(err, extension.ErrNotFound) {
				t.Errorf("%s: Responsible err = %v, want ErrNotFound", name, err)
			}
			return nil
		})
	}
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		if _, err := rt.Deals().Get(ctx, "not-a-deal"); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("a malformed id: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestTheResponsibleAreTheOwnerTheAssigneesAndTheAssignedTeamsLiveMembers(t *testing.T) {
	s := setupSystemRead(t)
	desk, closed := ids.NewV7(), ids.NewV7()
	s.WsExec(t, `INSERT INTO team (id, name) VALUES ($1, 'Fleet desk'), ($2, 'Closed desk')`, desk, closed)
	live := s.member(t, desk, "live@example.test", "active", false)
	gone := s.member(t, desk, "gone@example.test", "deactivated", false)
	agent := s.member(t, desk, "agent@example.test", "active", true)
	fromClosed := s.member(t, closed, "closed@example.test", "active", false)
	s.assign(t, crmcontracts.AssignmentSubjectKindTeam, desk)
	s.assign(t, crmcontracts.AssignmentSubjectKindTeam, closed)
	s.WsExec(t, `UPDATE team SET archived_at = now() WHERE id = $1`, closed)

	got := s.responsible(t)
	if len(got) == 0 || got[0] != (extension.Responsible{UserID: asUser(s.AdminUser), Owner: true}) {
		t.Fatalf("responsible = %+v, want the owner first", got)
	}
	who := userIDs(got)
	for _, want := range []ids.UUID{s.Rep1, s.Rep2, live} {
		if !slices.Contains(who, asUser(want)) {
			t.Errorf("%s is responsible for the deal and was left out of %v", want, who)
		}
	}
	for name, unwanted := range map[string]ids.UUID{"a deactivated member": gone, "an agent": agent, "an archived team's member": fromClosed} {
		if slices.Contains(who, asUser(unwanted)) {
			t.Errorf("%s was named responsible", name)
		}
	}
	if owners := slices.DeleteFunc(slices.Clone(got), func(r extension.Responsible) bool { return !r.Owner }); len(owners) != 1 {
		t.Errorf("%d colleagues were marked owner, want 1", len(owners))
	}
}

func TestCanReadAnswersUnderTheColleaguesOwnGrants(t *testing.T) {
	s := setupSystemRead(t)
	if !s.canRead(t, s.Rep1, dealCardFields...) {
		t.Error("a colleague who may read every deal was refused")
	}
	if s.canRead(t, s.Rep2, "name") {
		t.Error("a colleague with no deal grant may read the deal")
	}
	s.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, s.Rep1)
	if s.canRead(t, s.Rep1, "name") {
		t.Error("a colleague who can no longer sign in may read the deal")
	}
}

// An agent seat is no colleague a unit may show a deal to, whatever it holds.
func TestCanReadAnswersNoForAnAgentSeat(t *testing.T) {
	s := setupSystemRead(t)
	agent := s.seat(t, "reader-agent@example.test", "active", true)
	s.grant(t, agent, dealReaderGrants)
	if s.canRead(t, agent, "name") {
		t.Error("an agent seat that may read every deal was answered as a colleague who may read it")
	}
}

func TestAnOwnerWhoseRoleMasksTheAmountMayNotReadIt(t *testing.T) {
	s := setupSystemRead(t)
	// A mask binds only a bounded role, so Rep3 reads its own deals alone, and
	// sees this one by owning it.
	role := "deal-reader-" + s.Rep3.String()
	s.WsExec(t, `UPDATE role SET permissions = '{"objects":{"deal":{"read":true}},"row_scope":"own"}' WHERE key = $1`, role)
	s.WsExec(t, `INSERT INTO field_mask (role_key, object, field, condition) VALUES ($1, 'deal', 'amount_minor', 'always')`, role)
	owner := ids.From[ids.UserKind](s.Rep3)
	if _, err := s.Deals.UpdateDeal(s.Admin(), ids.From[ids.DealKind](s.deal), deals.UpdateDealInput{OwnerID: &owner}); err != nil {
		t.Fatal(err)
	}

	if !s.canRead(t, s.Rep3, "name", "stage_id", "owner_id") {
		t.Error("the owner was refused fields no mask withholds")
	}
	for _, withheld := range []string{"amount_minor", "currency"} {
		if s.canRead(t, s.Rep3, "name", withheld) {
			t.Errorf("a colleague whose role masks the amount may read %s", withheld)
		}
	}
	if s.canRead(t, s.Rep3, dealCardFields...) {
		t.Error("a colleague whose role masks the amount may read every card field")
	}
}

func TestCanReadRefusesAQuestionItCannotAnswer(t *testing.T) {
	s := setupSystemRead(t)
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		for name, ask := range map[string]struct {
			record extension.EntityRef
			fields []string
		}{
			"an unknown field": {extension.EntityRef{Type: "deal", ID: s.deal.String()}, []string{"amount"}},
			"another type":     {extension.EntityRef{Type: "contact", ID: s.deal.String()}, []string{"name"}},
			"a malformed id":   {extension.EntityRef{Type: "deal", ID: "deal-1"}, []string{"name"}},
		} {
			if _, err := rt.Authority().CanRead(ctx, asUser(s.Rep1), ask.record, ask.fields); !errors.Is(err, extension.ErrInvalid) {
				t.Errorf("%s: err = %v, want ErrInvalid", name, err)
			}
		}
		return nil
	})
}

// A unit that invites at most a provider's cap of colleagues probes the
// responsible list in order. Colleagues the probe refuses must not use up the
// list before one it admits.
func TestColleaguesTheProbeRefusesDoNotCrowdOutOneItAdmits(t *testing.T) {
	s := setupSystemRead(t)
	const invitesPerCall = 100
	desk := ids.NewV7()
	s.WsExec(t, `INSERT INTO team (id, name) VALUES ($1, 'Fleet desk')`, desk)
	for i := range invitesPerCall + 10 {
		s.member(t, desk, fmt.Sprintf("0desk-%03d@example.test", i), "active", false)
	}
	s.WsExec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, desk, s.Rep3)
	s.assign(t, crmcontracts.AssignmentSubjectKindTeam, desk)

	var invited []extension.UserID
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		responsible, err := rt.Deals().Responsible(ctx, s.deal.String())
		if err != nil {
			return err
		}
		for _, r := range responsible {
			if len(invited) == invitesPerCall {
				break
			}
			shown, err := rt.Authority().CanRead(ctx, r.UserID, extension.EntityRef{Type: "deal", ID: s.deal.String()}, dealCardFields)
			if err != nil {
				return err
			}
			if shown {
				invited = append(invited, r.UserID)
			}
		}
		return nil
	})
	if !slices.Contains(invited, asUser(s.Rep3)) {
		t.Errorf("a team member who may open the deal was crowded out by %d who may not", invitesPerCall+10)
	}
}

func TestEmailsAnswerActiveHumanMembersOnly(t *testing.T) {
	s := setupSystemRead(t)
	s.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, s.Rep2)
	agent := s.seat(t, "mail-agent@example.test", "active", true)
	rep1 := s.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, s.Rep1)
	var got map[extension.UserID]string
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		got, err = rt.Directory().Emails(ctx,
			[]extension.UserID{asUser(s.Rep1), asUser(s.Rep2), asUser(agent), asUser(ids.NewV7())})
		return err
	})
	if len(got) != 1 || got[asUser(s.Rep1)] != rep1 {
		t.Errorf("emails = %v, want only %s's address %s", got, s.Rep1, rep1)
	}

	tooMany := make([]extension.UserID, extension.MaxResponsible+1)
	for i := range tooMany {
		tooMany[i] = asUser(ids.NewV7())
	}
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		if _, err := rt.Directory().Emails(ctx, tooMany); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("%d users: err = %v, want ErrInvalid", len(tooMany), err)
		}
		return nil
	})
}

// On a call somebody is behind, a system read could hand that caller what their
// own grants withhold, so every one of them refuses.
func TestASystemReadIsRefusedOnACallThatHasACaller(t *testing.T) {
	s := setupSystemRead(t)
	rt := runtimeFor(s.Admin(), "dealmirror", "1.0.0", "tool/probe", boundExtensionRuntime())
	defer rt.release()
	ctx := context.Background()
	deal := extension.EntityRef{Type: "deal", ID: s.deal.String()}
	refusals := map[string]error{}
	_, refusals["Deals().Get"] = rt.Deals().Get(ctx, s.deal.String())
	_, refusals["Deals().Responsible"] = rt.Deals().Responsible(ctx, s.deal.String())
	_, refusals["Authority().CanRead"] = rt.Authority().CanRead(ctx, asUser(s.Rep1), deal, []string{"name"})
	_, refusals["Directory().Emails"] = rt.Directory().Emails(ctx, []extension.UserID{asUser(s.Rep1)})
	for read, err := range refusals {
		if !errors.Is(err, extension.ErrAttendedRead) {
			t.Errorf("%s on an attended call: err = %v, want ErrAttendedRead", read, err)
		}
	}
}

// A signed-in human may ask about themselves, and the answer is their own
// grants', not the permissions the request carried.
func TestAnAttendedCanReadAnswersTheCallerUnderTheirOwnGrants(t *testing.T) {
	s := setupSystemRead(t)
	deal := extension.EntityRef{Type: "deal", ID: s.deal.String()}
	for user, want := range map[ids.UUID]bool{s.Rep1: true, s.Rep2: false} {
		rt := runtimeFor(s.As(user, nil, integration.AdminPerms), "dealmirror", "1.0.0", "tool/probe", boundExtensionRuntime())
		shown, err := rt.Authority().CanRead(context.Background(), asUser(user), deal, []string{"name"})
		rt.release()
		if err != nil {
			t.Fatalf("CanRead about the caller: %v", err)
		}
		if shown != want {
			t.Errorf("CanRead about %s = %v, want %v", user, shown, want)
		}
	}
}

// A deal id no row holds is no deal anyone may open, even for a colleague
// whose row scope is every deal.
func TestCanReadAnswersNoForADealThatDoesNotExist(t *testing.T) {
	s := setupSystemRead(t)
	var shown bool
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		shown, err = rt.Authority().CanRead(ctx, asUser(s.Rep1),
			extension.EntityRef{Type: "deal", ID: ids.NewV7().String()}, []string{"name"})
		return err
	})
	if shown {
		t.Error("a colleague who may read every deal may read one that does not exist")
	}
}

// dealWriterGrants lets its holder change every deal.
const dealWriterGrants = `{"objects":{"deal":{"read":true,"update":true}},"row_scope":"all"}`

func (s *systemReadEnv) canWrite(t *testing.T, user ids.UUID, deal ids.UUID) bool {
	t.Helper()
	var writable bool
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		writable, err = rt.Authority().CanWrite(ctx, asUser(user), extension.EntityRef{Type: "deal", ID: deal.String()})
		return err
	})
	return writable
}

func TestCanWriteAnswersUnderTheColleaguesOwnGrants(t *testing.T) {
	s := setupSystemRead(t)
	writer := s.seat(t, "writer@example.test", "active", false)
	s.grant(t, writer, dealWriterGrants)
	if !s.canWrite(t, writer, s.deal) {
		t.Error("a colleague who may change every deal was refused")
	}
	if s.canWrite(t, s.Rep1, s.deal) {
		t.Error("a colleague who may only read deals may change one")
	}
	if s.canWrite(t, writer, ids.NewV7()) {
		t.Error("a deal no row holds was answered as one a writer may change")
	}
	s.WsExec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, s.deal)
	if s.canWrite(t, writer, s.deal) {
		t.Error("an archived deal was answered as one a writer may change")
	}
}

// The write scope and both verbs decide, not the update verb alone.
func TestCanWriteNeedsTheRowScopeAndTheReadVerb(t *testing.T) {
	s := setupSystemRead(t)
	ownOnly := s.seat(t, "own@example.test", "active", false)
	s.grant(t, ownOnly, `{"objects":{"deal":{"read":true,"update":true}},"row_scope":"own"}`)
	if s.canWrite(t, ownOnly, s.deal) {
		t.Error("a writer of their own deals may change a deal someone else owns")
	}
	blind := s.seat(t, "blind@example.test", "active", false)
	s.grant(t, blind, `{"objects":{"deal":{"update":true}},"row_scope":"all"}`)
	if s.canWrite(t, blind, s.deal) {
		t.Error("a role with update but no read on deals may change one")
	}
}

// A read seat changes nothing, whatever its role holds, even on its own deal.
func TestCanWriteRefusesAReadSeat(t *testing.T) {
	s := setupSystemRead(t)
	reader := s.seat(t, "reader@example.test", "active", false)
	s.grant(t, reader, dealWriterGrants)
	s.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, reader)
	if s.canWrite(t, reader, s.deal) {
		t.Error("a read seat whose role may change every deal may change one")
	}
	owner := s.seat(t, "owner@example.test", "active", false)
	s.grant(t, owner, `{"objects":{"deal":{"read":true,"update":true}},"row_scope":"own"}`)
	s.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, owner)
	s.WsExec(t, `UPDATE deal SET owner_id = $1 WHERE id = $2`, owner, s.deal)
	if s.canWrite(t, owner, s.deal) {
		t.Error("a read seat may change the deal it owns")
	}
}

// On an attended call CanWrite answers its human caller about themselves.
func TestAnAttendedCanWriteAnswersTheCaller(t *testing.T) {
	s := setupSystemRead(t)
	writer := s.seat(t, "writer@example.test", "active", false)
	s.grant(t, writer, dealWriterGrants)
	deal := extension.EntityRef{Type: "deal", ID: s.deal.String()}
	for user, want := range map[ids.UUID]bool{writer: true, s.Rep1: false} {
		rt := runtimeFor(s.As(user, nil, integration.AdminPerms), "dealmirror", "1.0.0", "tool/probe", boundExtensionRuntime())
		writable, err := rt.Authority().CanWrite(context.Background(), asUser(user), deal)
		rt.release()
		if err != nil {
			t.Fatalf("CanWrite about the caller: %v", err)
		}
		if writable != want {
			t.Errorf("CanWrite about %s = %v, want %v", user, writable, want)
		}
	}
	rt := runtimeFor(s.As(writer, nil, integration.AdminPerms), "dealmirror", "1.0.0", "tool/probe", boundExtensionRuntime())
	defer rt.release()
	if _, err := rt.Authority().CanWrite(context.Background(), asUser(s.Rep1), deal); !errors.Is(err, extension.ErrAttendedRead) {
		t.Errorf("CanWrite about a colleague on an attended call: err = %v, want ErrAttendedRead", err)
	}
}
