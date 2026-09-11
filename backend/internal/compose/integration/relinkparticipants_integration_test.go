// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Relink repoints the activity's PARTICIPANTS along with its link, so the two
// keep telling one story: a human saying "this conversation was not with her,
// it was with him" must not leave her named on it. activity_participant is
// registered PII and is what the interaction-edge projection derives its
// (user, contact) pairs from, so a row left behind keeps feeding a
// relationship-strength signal for somebody the human just ruled out.
//
// Both shapes below used to break that, in opposite directions.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seedRelinkActivity(t *testing.T, e *Env, subject string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', $2, now(), 'manual', 'human:x')`, id, subject)
	return id
}

func namedOn(t *testing.T, e *Env, activity, contact ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity_participant
		 WHERE activity_id = $1 AND contact_id = $2`, activity, contact)
}

// The target is ALREADY a participant. The repoint's skip left the displaced
// row standing, so the activity's link named him and its participants still
// named her.
func TestRelinkRemovesTheDisplacedParticipantWhenTheTargetIsAlreadyOne(t *testing.T) {
	e := Setup(t)
	admin := e.As(e.Rep1, nil, AdminPerms)
	her, him := e.SeedContact(t, "Ingrid Sattler", nil), e.SeedContact(t, "Tomas Berg", nil)
	act := seedRelinkActivity(t, e, "Quote follow-up")
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, act, her)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, contact_id) VALUES ($1, 'to', $2)`, act, her)
	// He was on it too, in ANOTHER role — a different row under
	// uq_activity_participant, and no obstacle to the repoint, but enough to
	// make the old guard skip it because that guard asked only about the
	// contact. The displaced row is promoted here; the same-role case below is
	// the one that has to delete instead.
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, contact_id) VALUES ($1, 'cc', $2)`, act, him)

	if _, err := e.Activities.RelinkActivity(admin, ids.From[ids.ActivityKind](act), activities.RelinkActivityInput{
		EntityType: "contact", EntityID: him, ReplaceExistingOfType: true,
	}); err != nil {
		t.Fatalf("relink: %v", err)
	}

	if n := namedOn(t, e, act, her); n != 0 {
		t.Errorf("the displaced contact is still named on the activity (%d row(s)) — "+
			"its link says one thing and its participants another", n)
	}
	if n := namedOn(t, e, act, him); n == 0 {
		t.Error("the target is named on no participant row; the relink dropped the conversation's only party")
	}
}

// SEVERAL displaced participants and no target row. Every one of them
// qualified for the rewrite, and the second collided with the first on
// uq_activity_participant.
func TestRelinkMergesSeveralDisplacedParticipantsWithoutColliding(t *testing.T) {
	e := Setup(t)
	admin := e.As(e.Rep1, nil, AdminPerms)
	her, other := e.SeedContact(t, "Ingrid Sattler", nil), e.SeedContact(t, "Petra Lang", nil)
	him := e.SeedContact(t, "Tomas Berg", nil)
	act := seedRelinkActivity(t, e, "Renewal thread")
	for _, p := range []ids.UUID{her, other} {
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, act, p)
		// The SAME role, which is what makes the two collide once both are
		// rewritten to one contact.
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, contact_id) VALUES ($1, 'to', $2)`, act, p)
	}

	if _, err := e.Activities.RelinkActivity(admin, ids.From[ids.ActivityKind](act), activities.RelinkActivityInput{
		EntityType: "contact", EntityID: him, ReplaceExistingOfType: true,
	}); err != nil {
		t.Fatalf("relink with two displaced participants: %v", err)
	}

	if n := namedOn(t, e, act, him); n != 1 {
		t.Errorf("the target is named on %d participant row(s), want exactly 1 — "+
			"two displaced rows merge onto one contact, they do not each become one", n)
	}
	for name, p := range map[string]ids.UUID{"Ingrid": her, "Petra": other} {
		if n := namedOn(t, e, act, p); n != 0 {
			t.Errorf("%s is still named on the activity (%d row(s)) after being relinked away", name, n)
		}
	}
}

// The target already holds a row of the SAME shape, so there is nothing to
// promote the displaced row into — it has to be deleted. This is the branch
// the different-role case above cannot reach, and the one that decides whether
// "merge" means merge or means duplicate.
func TestRelinkDeletesTheDisplacedParticipantWhenTheTargetHoldsItsShape(t *testing.T) {
	e := Setup(t)
	admin := e.As(e.Rep1, nil, AdminPerms)
	her, him := e.SeedContact(t, "Ingrid Sattler", nil), e.SeedContact(t, "Tomas Berg", nil)
	act := seedRelinkActivity(t, e, "Quote follow-up")
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, act, her)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, contact_id) VALUES ($1, 'to', $2)`, act, her)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, contact_id) VALUES ($1, 'to', $2)`, act, him)

	if _, err := e.Activities.RelinkActivity(admin, ids.From[ids.ActivityKind](act), activities.RelinkActivityInput{
		EntityType: "contact", EntityID: him, ReplaceExistingOfType: true,
	}); err != nil {
		t.Fatalf("relink: %v", err)
	}

	if n := namedOn(t, e, act, her); n != 0 {
		t.Errorf("the displaced contact is still named on the activity (%d row(s))", n)
	}
	if n := namedOn(t, e, act, him); n != 1 {
		t.Errorf("the target is named on %d row(s), want exactly 1 — its own row was already there, "+
			"so the displaced one is removed rather than renamed onto it", n)
	}
}
