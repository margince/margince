// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

// Coaching against a real database: who may raise one, for whom, and what the
// recipient ends up holding.
//
// Two gates guard this, and each is held on its own. The ROLE gate says whether
// a seat coaches at all; the TEAMMATE gate says whom. Testing only their
// conjunction would let either one rot silently — a role check that admitted
// everybody would still look correct while the membership check carried the
// whole weight, and the reverse.

import (
	"context"
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// asRole is asUser with role keys, which is what the coaching gate reads.
func (e *noticeEnv) asRole(u ids.UserID, roles ...string) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + u.String(), UserID: u.UUID,
		Permissions: principal.Permissions{RoleKeys: roles},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// teammatesSaying answers every membership question the same way. The gate
// under test is the store's use of the answer, not the answer itself — the
// identity module holds that against real rows.
type teammatesSaying bool

func (t teammatesSaying) SharesLiveTeamWithCaller(context.Context, ids.UUID) (bool, error) {
	return bool(t), nil
}

func TestALeadCoachesATeammateAndTheNoticeIsTheirs(t *testing.T) {
	e := setupNotices(t)
	lead, rep := e.other, e.recipient

	notice, err := e.store.RaiseCoachNotice(
		e.asRole(lead, "manager"), teammatesSaying(true), rep,
		crmcontracts.CoachReplyAging, "  Kirsten has been waiting since Tuesday.  ")
	if err != nil {
		t.Fatalf("a Team Lead coaching their teammate: %v", err)
	}

	// The KIND supplies the headline; the coach supplies only the note, trimmed.
	if notice.Subject == "" {
		t.Fatal("the notice reached its recipient with no headline")
	}
	if notice.Body != "Kirsten has been waiting since Tuesday." {
		t.Fatalf("the note came back as %q", notice.Body)
	}
	if notice.Kind != string(crmcontracts.CoachReplyAging) {
		t.Fatalf("the notice recorded kind %q", notice.Kind)
	}
	if notice.CreatedAt.IsZero() {
		t.Fatal("the notice carries no creation time, so the client cannot order it")
	}

	// It is the RECIPIENT's, not the coach's. A notice that landed in the
	// raiser's own queue would read as somebody asking THEM for something.
	theirs, err := e.store.UnreadFor(e.asUser(rep), 10)
	if err != nil {
		t.Fatalf("the recipient reading their notices: %v", err)
	}
	if len(theirs) != 1 || theirs[0].ID != notice.ID {
		t.Fatalf("the recipient holds %d notices, wanted the one raised for them", len(theirs))
	}
	coachs, err := e.store.UnreadFor(e.asRole(lead, "manager"), 10)
	if err != nil {
		t.Fatalf("the coach reading their own notices: %v", err)
	}
	if len(coachs) != 0 {
		t.Fatalf("the coach holds %d notices of their own after raising one for somebody else", len(coachs))
	}
}

// The ROLE gate on its own: a rep is refused even for somebody they share a
// team with. Coaching is a thing a lead does.
func TestARepDoesNotCoachEvenATeammate(t *testing.T) {
	e := setupNotices(t)

	_, err := e.store.RaiseCoachNotice(
		e.asRole(e.other, "rep"), teammatesSaying(true), e.recipient,
		crmcontracts.CoachGeneral, "a word")
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep coaching a teammate got %v, wanted a refusal", err)
	}

	// And the admit case beside it, over the SAME membership answer, so the
	// refusal above is about the role and nothing else.
	if _, err := e.store.RaiseCoachNotice(
		e.asRole(e.other, "manager"), teammatesSaying(true), e.recipient,
		crmcontracts.CoachGeneral, "a word"); err != nil {
		t.Fatalf("a Team Lead over the same membership answer was refused: %v", err)
	}
}

// The TEAMMATE gate on its own: a lead is refused for somebody on no team of
// theirs, over the same role that just succeeded.
func TestALeadDoesNotCoachSomebodyOnAnotherTeam(t *testing.T) {
	e := setupNotices(t)

	_, err := e.store.RaiseCoachNotice(
		e.asRole(e.other, "manager"), teammatesSaying(false), e.recipient,
		crmcontracts.CoachGeneral, "a word")
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a lead coaching a stranger got %v, wanted a refusal", err)
	}
}

// A system flow raises its own kinds through the notifier seam. One arriving
// here would be a background pass writing in a person's voice.
func TestASystemPassDoesNotCoach(t *testing.T) {
	e := setupNotices(t)

	_, err := e.store.RaiseCoachNotice(
		e.engineCtx(), teammatesSaying(true), e.recipient,
		crmcontracts.CoachGeneral, "a word")
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the automation engine coaching a person got %v, wanted a refusal", err)
	}
}

// A kind outside the vocabulary is a malformed request, not a permission
// decision: the caller may well coach, and asked for something that is not a
// coaching notice.
func TestAnUnknownKindIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	e := setupNotices(t)

	_, err := e.store.RaiseCoachNotice(
		e.asRole(e.other, "manager"), teammatesSaying(true), e.recipient,
		crmcontracts.NoticeKind("automation"), "a word")
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Field != "kind" {
		t.Fatalf("raising an automation notice by hand got %v, wanted a validation error on kind", err)
	}

	// Nothing was written. A refusal that left a row behind would put an
	// unheadlined notice in somebody's queue.
	held, err := e.store.UnreadFor(e.asUser(e.recipient), 10)
	if err != nil {
		t.Fatalf("reading the recipient's notices: %v", err)
	}
	if len(held) != 0 {
		t.Fatalf("a refused raise left %d notices behind", len(held))
	}
}

// A note past its ceiling is refused rather than silently truncated: the coach
// would otherwise believe they had said something the recipient never reads.
func TestAnOversizeNoteIsRefusedRatherThanTrimmed(t *testing.T) {
	e := setupNotices(t)

	_, err := e.store.RaiseCoachNotice(
		e.asRole(e.other, "manager"), teammatesSaying(true), e.recipient,
		crmcontracts.CoachGeneral, strings.Repeat("x", noteBound+1))
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Field != "note" {
		t.Fatalf("an oversize note got %v, wanted a validation error on note", err)
	}

	// The ceiling itself is admitted, or the bound is off by one in the
	// direction nobody notices.
	if _, err := e.store.RaiseCoachNotice(
		e.asRole(e.other, "manager"), teammatesSaying(true), e.recipient,
		crmcontracts.CoachGeneral, strings.Repeat("x", noteBound)); err != nil {
		t.Fatalf("a note exactly at the ceiling was refused: %v", err)
	}
}

// Coaching yourself is not a notice. It would sit in the raiser's own queue
// looking like somebody had asked them for something.
func TestCoachingYourselfIsRefused(t *testing.T) {
	e := setupNotices(t)

	_, err := e.store.RaiseCoachNotice(
		e.asRole(e.other, "manager"), teammatesSaying(true), e.other,
		crmcontracts.CoachGeneral, "a word")
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Field != "recipient_user_id" {
		t.Fatalf("coaching yourself got %v, wanted a validation error on the recipient", err)
	}
}

// A notice with no note still says what it is about, which is why the kind is
// closed and the note is not required.
func TestANoticeWithNoNoteStillSaysWhatItIsAbout(t *testing.T) {
	e := setupNotices(t)

	notice, err := e.store.RaiseCoachNotice(
		e.asRole(e.other, "manager"), teammatesSaying(true), e.recipient,
		crmcontracts.CoachReviewBacklog, "")
	if err != nil {
		t.Fatalf("raising a notice with no note: %v", err)
	}
	if notice.Subject == "" {
		t.Fatal("a notice with no note reached its recipient saying nothing at all")
	}
	if notice.Body != "" {
		t.Fatalf("a notice raised with no note carries body %q", notice.Body)
	}
}
