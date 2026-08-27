// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Relink repoints the activity's PARTICIPANTS along with its link, so the two
// keep telling one story: a human saying "this conversation was not with her,
// it was with him" must not leave her named on it. activity_participant is
// registered PII and is what the interaction-edge projection derives its
// (user, person) pairs from, so a row left behind keeps feeding a
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

func namedOn(t *testing.T, e *Env, activity, person ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity_participant
		 WHERE activity_id = $1 AND person_id = $2`, activity, person)
}

// The target is ALREADY a participant. The repoint's skip left the displaced
// row standing, so the activity's link named him and its participants still
// named her.
func TestRelinkRemovesTheDisplacedParticipantWhenTheTargetIsAlreadyOne(t *testing.T) {
	e := Setup(t)
	admin := e.As(e.Rep1, nil, AdminPerms)
	her, him := e.SeedPerson(t, "Ingrid Sattler", nil), e.SeedPerson(t, "Tomas Berg", nil)
	act := seedRelinkActivity(t, e, "Quote follow-up")
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, person_id) VALUES ($1, 'person', $2)`, act, her)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, person_id) VALUES ($1, 'to', $2)`, act, her)
	// He was on it too, in another role — which is a different row under
	// uq_activity_participant, and used to be enough to skip the repoint.
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, person_id) VALUES ($1, 'cc', $2)`, act, him)

	if _, err := e.Activities.RelinkActivity(admin, ids.From[ids.ActivityKind](act), activities.RelinkActivityInput{
		EntityType: "person", EntityID: him, ReplaceExistingOfType: true,
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
	her, other := e.SeedPerson(t, "Ingrid Sattler", nil), e.SeedPerson(t, "Petra Lang", nil)
	him := e.SeedPerson(t, "Tomas Berg", nil)
	act := seedRelinkActivity(t, e, "Renewal thread")
	for _, p := range []ids.UUID{her, other} {
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, person_id) VALUES ($1, 'person', $2)`, act, p)
		// The SAME role, which is what makes the two collide once both are
		// rewritten to one person.
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, person_id) VALUES ($1, 'to', $2)`, act, p)
	}

	if _, err := e.Activities.RelinkActivity(admin, ids.From[ids.ActivityKind](act), activities.RelinkActivityInput{
		EntityType: "person", EntityID: him, ReplaceExistingOfType: true,
	}); err != nil {
		t.Fatalf("relink with two displaced participants: %v", err)
	}

	if n := namedOn(t, e, act, him); n != 1 {
		t.Errorf("the target is named on %d participant row(s), want exactly 1 — "+
			"two displaced rows merge onto one person, they do not each become one", n)
	}
	for name, p := range map[string]ids.UUID{"Ingrid": her, "Petra": other} {
		if n := namedOn(t, e, act, p); n != 0 {
			t.Errorf("%s is still named on the activity (%d row(s)) after being relinked away", name, n)
		}
	}
}
