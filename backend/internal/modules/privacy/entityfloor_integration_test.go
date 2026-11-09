// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// entityFloorFixture is one installation with a default entity in one country
// and a further entity in another. Its colleague works for the further one.
// Its messages are commercial correspondence, 400 days old.
type entityFloorFixture struct {
	tx        pgx.Tx
	colleague ids.UUID
	svc       *RetentionService
}

func newEntityFloorFixture(ctx context.Context, t *testing.T, defaultCountry, furtherCountry jurisdiction.Code) entityFloorFixture {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	})
	country := defaultCountry
	svc := &RetentionService{eraser: NewEraser(nil).WithCountry(func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return country, nil })}
	f := entityFloorFixture{tx: tx, svc: svc}
	f.colleague = f.member(ctx, t, furtherCountry)
	return f
}

// member adds a user who works for a new further entity in country, or for
// the default entity when country is empty.
func (f entityFloorFixture) member(ctx context.Context, t *testing.T, country jurisdiction.Code) ids.UUID {
	t.Helper()
	user := ids.NewV7()
	mustExec(ctx, t, f.tx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
		user, "colleague-"+user.String()+"@floor.test")
	if country == "" {
		return user
	}
	entity := ids.NewV7()
	mustExec(ctx, t, f.tx, `INSERT INTO legal_entity (id, display_name, legal_name, country, captured_by)
		VALUES ($1, $2, 'Further Entity GmbH', $3, 'system:test')`, entity, "Further "+entity.String(), string(country))
	mustExec(ctx, t, f.tx, `INSERT INTO legal_entity_member (user_id, legal_entity_id, assigned_by) VALUES ($1, $2, 'system:test')`,
		user, entity)
	return user
}

// received files the message into user's mailbox.
func (f entityFloorFixture) received(ctx context.Context, t *testing.T, activity, user ids.UUID) {
	t.Helper()
	mustExec(ctx, t, f.tx, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, activity, user)
}

// mail writes one 400-day-old Handelsbrief with no users of its own.
func (f entityFloorFixture) mail(ctx context.Context, t *testing.T, direction string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	mustExec(ctx, t, f.tx, `INSERT INTO activity (id, kind, subject, body, direction, occurred_at, source, captured_by, retention_class, retention_class_at)
		VALUES ($1, 'email', 'Order', 'commercial content', $2, now() - interval '400 days', 'capture_email', 'connector:t', 'commercial_correspondence', now())`,
		id, direction)
	return id
}

func (f entityFloorFixture) kept(ctx context.Context, t *testing.T, activity ids.UUID) bool {
	t.Helper()
	err := f.svc.refuseIfShieldedNow(ctx, f.tx, activity)
	if err != nil && !errors.Is(err, errKeptByFloor) {
		t.Fatalf("asking the floor: %v", err)
	}
	return err != nil
}

// The default entity keeps no floor and the further entity a six-year one. A
// message is kept when it belongs to the further entity's colleague.
func TestAMessageIsKeptUnderTheFloorOfItsUsersEntity(t *testing.T) {
	ctx := context.Background()
	f := newEntityFloorFixture(ctx, t, "zv", "zu")

	sent := f.mail(ctx, t, "outbound")
	mustExec(ctx, t, f.tx, `UPDATE activity SET sent_by_colleague = $2 WHERE id = $1`, sent, f.colleague)

	received := f.mail(ctx, t, "inbound")
	f.received(ctx, t, received, f.colleague)

	logged := f.mail(ctx, t, "outbound")
	mustExec(ctx, t, f.tx, `UPDATE activity SET captured_by = 'human:' || $2::text WHERE id = $1`, logged, f.colleague)

	underDeal := f.mail(ctx, t, "inbound")
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, f.tx, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Floor', false, 99)`, pipeline)
	mustExec(ctx, t, f.tx, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability) VALUES ($1, $2, 'Open', 0, 'open', 10)`, stage, pipeline)
	mustExec(ctx, t, f.tx, `INSERT INTO deal (id, name, status, pipeline_id, stage_id, owner_id, source, captured_by)
		VALUES ($1, 'Owned deal', 'open', $2, $3, $4, 'api', 'system:test')`, deal, pipeline, stage, f.colleague)
	mustExec(ctx, t, f.tx, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, underDeal, deal)

	filed := f.mail(ctx, t, "inbound")
	contact := ids.NewV7()
	mustExec(ctx, t, f.tx, `INSERT INTO contact (id, full_name, source, owner_id, captured_by)
		VALUES ($1, 'Owned Contact', 'manual', $2, 'system:test')`, contact, f.colleague)
	mustExec(ctx, t, f.tx, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, filed, contact)

	nobodys := f.mail(ctx, t, "inbound")

	for name, c := range map[string]struct {
		activity ids.UUID
		kept     bool
	}{
		"sent by the colleague":           {sent, true},
		"received in the colleague's box": {received, true},
		"filed under their contact":       {filed, true},
		"logged by the colleague's seat":  {logged, true},
		"filed under their deal":          {underDeal, true},
		"naming nobody":                   {nobodys, false},
	} {
		if got := f.kept(ctx, t, c.activity); got != c.kept {
			t.Errorf("a message %s: kept=%t, want %t", name, got, c.kept)
		}
	}
}

// The default entity keeps six years and the further entity none. A message
// never keeps less than the default floor, so the colleague's mail stays.
func TestAFurtherEntityWithoutAFloorKeepsTheDefaultFloor(t *testing.T) {
	ctx := context.Background()
	f := newEntityFloorFixture(ctx, t, "zu", "zv")

	sent := f.mail(ctx, t, "outbound")
	mustExec(ctx, t, f.tx, `UPDATE activity SET sent_by_colleague = $2 WHERE id = $1`, sent, f.colleague)
	nobodys := f.mail(ctx, t, "inbound")

	if !f.kept(ctx, t, sent) {
		t.Error("the colleague's sent mail is released, want it kept under the default entity's floor")
	}
	if !f.kept(ctx, t, nobodys) {
		t.Error("a message naming nobody is released, want it kept under the default entity's floor")
	}
}

// A message several users received keeps the longest floor among their
// entities.
func TestTheLongestFloorAmongAMessagesUsersWins(t *testing.T) {
	ctx := context.Background()
	f := newEntityFloorFixture(ctx, t, "zv", "zu")
	noFloor := f.member(ctx, t, "zv")
	inDefault := f.member(ctx, t, "")

	shared := f.mail(ctx, t, "inbound")
	for _, user := range []ids.UUID{noFloor, inDefault, f.colleague} {
		f.received(ctx, t, shared, user)
	}
	notOurs := f.mail(ctx, t, "inbound")
	f.received(ctx, t, notOurs, noFloor)
	f.received(ctx, t, notOurs, inDefault)

	if !f.kept(ctx, t, shared) {
		t.Error("a message one six-year colleague received is released, want the longest floor to keep it")
	}
	if f.kept(ctx, t, notOurs) {
		t.Error("a message only no-floor users received is kept, want it released")
	}
}

// The owners of the records a message is filed under count only when the
// message names no users of its own.
func TestARecordOwnerCountsOnlyForAMessageThatNamesNobody(t *testing.T) {
	ctx := context.Background()
	f := newEntityFloorFixture(ctx, t, "zv", "zu")
	inDefault := f.member(ctx, t, "")

	received := f.mail(ctx, t, "inbound")
	f.received(ctx, t, received, inDefault)
	contact := ids.NewV7()
	mustExec(ctx, t, f.tx, `INSERT INTO contact (id, full_name, source, owner_id, captured_by)
		VALUES ($1, 'Owned Contact', 'manual', $2, 'system:test')`, contact, f.colleague)
	mustExec(ctx, t, f.tx, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, received, contact)

	if f.kept(ctx, t, received) {
		t.Error("a message in a default-entity mailbox is kept by its contact owner's entity, want its own user to decide")
	}
}

// tenYearPack keeps commercial correspondence ten years, longer than zu.
type tenYearPack struct{}

func (tenYearPack) Code() jurisdiction.Code { return "zw" }

func (tenYearPack) Retention() jurisdiction.Retention { return tenYearClasses{} }

type tenYearClasses struct{}

func (tenYearClasses) Classes() []jurisdiction.RetentionClass {
	return []jurisdiction.RetentionClass{{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 10}}}
}

func init() { jurisdiction.Register(tenYearPack{}) }

// A report about some messages names the floor of their users' entities, not
// the longest floor any entity of the installation keeps.
func TestAReportNamesTheFloorOfItsOwnMessages(t *testing.T) {
	ctx := context.Background()
	f := newEntityFloorFixture(ctx, t, "zv", "zu")
	f.member(ctx, t, "zw")
	ours := f.mail(ctx, t, "inbound")
	f.received(ctx, t, ours, f.colleague)

	policy, err := ResolveFloorPolicy(ctx, f.tx, f.svc.eraser.country)
	if err != nil {
		t.Fatal(err)
	}
	if got := policy.StrictestClass(clocktest.Now(t)).Keep; got != (jurisdiction.Period{Years: 10}) {
		t.Fatalf("the installation's strictest floor keeps %v, want the ten-year entity's", got)
	}
	narrowed, err := policy.ForMessagesTx(ctx, f.tx, []ids.UUID{ours})
	if err != nil {
		t.Fatal(err)
	}
	if got := narrowed.StrictestClass(clocktest.Now(t)).Keep; got != (jurisdiction.Period{Years: 6}) {
		t.Errorf("a report on the colleague's message names a floor of %v, want their entity's six years", got)
	}
}
