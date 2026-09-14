// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Taking a disclosure duty and ending one without sending anything, against a
// real database — because every rule these writes hold is a CHECK constraint or
// a row lock, and neither exists in a fake.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// officerCtx is the env's context with the privacy-queue grant added.
//
// The shared harness seats a rep holding `contact`, which every other test in
// this package needs and none of these do: a notice case is gated on
// `privacy_request`, the same object the subject-request queue uses, so an
// installation that delegated its privacy inbox delegated this with it.
func officerCtx(e *channelConsentEnv) context.Context {
	actor, _ := principal.Actor(e.ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{
		"contact":         {Create: true, Read: true, Update: true, Delete: true},
		"privacy_request": {Create: true, Read: true, Update: true, Delete: true},
	}
	return principal.WithActor(e.ctx, actor)
}

// TestTakingADutyShowsWhoIsWorkingIt is the queue's whole reason for the
// assigned state: an owned case reads differently from an unclaimed one.
func TestTakingADutyShowsWhoIsWorkingIt(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	got, err := e.store.AssignNoticeCase(ctx, id, e.user)
	if err != nil {
		t.Fatalf("taking the duty: %v", err)
	}
	if got.State != NoticeAssigned {
		t.Errorf("an open duty somebody took is %q, want %q", got.State, NoticeAssigned)
	}
	if got.OwnerUserID == nil || *got.OwnerUserID != e.user {
		t.Errorf("the case names owner %v, want the seat that took it (%v)", got.OwnerUserID, e.user)
	}
	if got.AssignedAt == nil {
		t.Error("a taken case says when it was taken; assigned_at is null")
	}
}

// TestAnEndedDutyTakesNoOwner. Assigning a closed case would put it back on the
// queue as work somebody is doing, when the duty is over — the queue's own read
// filters on the unresolved states, so an assigned-and-completed row is a
// contradiction the state column cannot hold.
func TestAnEndedDutyTakesNoOwner(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	if _, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeExemptWithReason,
		Note:  "Art. 14(5)(b): notice to this contact is impossible, the address bounced permanently",
		Now:   time.Now(),
	}); err != nil {
		t.Fatalf("excusing the duty: %v", err)
	}

	var verr *ValidationError
	_, err := e.store.AssignNoticeCase(ctx, id, e.user)
	if !errors.As(err, &verr) || verr.Field != fieldState {
		t.Fatalf("assigning an ended duty answered %v, want a validation error on %q", err, fieldState)
	}
}

// TestExcusingADutyKeepsTheGround is the difference between these two states
// and not_required, which records the same conclusion with nothing to defend
// it. An auditor asking why a case is closed reads this column.
func TestExcusingADutyKeepsTheGround(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})
	when := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	const ground = "Told in person at the trade fair on the 3rd; they took the leaflet"

	got, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeProvidedElsewhere, Note: ground, Now: when,
	})
	if err != nil {
		t.Fatalf("recording that the duty was met elsewhere: %v", err)
	}
	if got.State != NoticeProvidedElsewhere {
		t.Errorf("the case is %q, want %q", got.State, NoticeProvidedElsewhere)
	}
	if got.ResolutionNote == nil || *got.ResolutionNote != ground {
		t.Errorf("the ground reads %v, want the officer's own words", got.ResolutionNote)
	}
	if got.ResolvedBy == nil || *got.ResolvedBy != e.user {
		t.Errorf("the case names resolver %v, want the seat that excused it (%v)", got.ResolvedBy, e.user)
	}
	// The caller's clock, not the database's: it is what lets a test drive a
	// deadline, and a completed_at read from now() would answer a different
	// question from the one this asked.
	if got.CompletedAt == nil || !got.CompletedAt.Equal(when) {
		t.Errorf("the case ended at %v, want the caller's own clock (%v)", got.CompletedAt, when)
	}
}

// TestAnExcuseWithoutAGroundIsRefused, including one made of whitespace — the
// table's CHECK sees a non-null value there and would let " " close a
// compliance record with nothing written on it.
func TestAnExcuseWithoutAGroundIsRefused(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)

	for _, note := range []string{"", "   \n\t "} {
		id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})
		var verr *ValidationError
		_, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
			State: NoticeExemptWithReason, Note: note, Now: time.Now(),
		})
		if !errors.As(err, &verr) || verr.Field != fieldResolutionNote {
			t.Errorf("excusing with note %q answered %v, want a validation error on %q",
				note, err, fieldResolutionNote)
		}
	}
}

// TestAGroundLongerThanTheColumnIsRefusedByName. The table holds this too, and
// a raw constraint violation reads as a database fault — the writer gets blamed
// for what is a bad argument.
func TestAGroundLongerThanTheColumnIsRefusedByName(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	var verr *ValidationError
	_, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeExemptWithReason,
		Note:  strings.Repeat("x", noticeNoteLimit+1),
		Now:   time.Now(),
	})
	if !errors.As(err, &verr) || verr.Field != fieldResolutionNote {
		t.Fatalf("an over-long ground answered %v, want a validation error on %q", err, fieldResolutionNote)
	}
}

// TestADischargedDutyIsNotExcusedAfterwards. A later note would overwrite a
// real discharge and then read as the reason it happened — the audit trail
// would say a mail went out because the subject already had the information.
func TestADischargedDutyIsNotExcusedAfterwards(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	if _, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeProvidedElsewhere, Note: "told by a colleague in their own mail", Now: time.Now(),
	}); err != nil {
		t.Fatalf("the first excuse: %v", err)
	}

	var verr *ValidationError
	_, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeExemptWithReason, Note: "actually it was exempt", Now: time.Now(),
	})
	if !errors.As(err, &verr) || verr.Field != fieldState {
		t.Fatalf("excusing an ended duty answered %v, want a validation error on %q", err, fieldState)
	}
}

// TestAnExcusedDutyLeavesTheQueue is what makes this a discharge rather than a
// note: the case stops being owed, which the queue derives from the terminal
// set rather than a second list.
func TestAnExcusedDutyLeavesTheQueue(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	before, _, err := e.store.ListNoticeCases(ctx, nil, 0, "")
	if err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	if len(before) != 1 || before[0].ID != id {
		t.Fatalf("the queue holds %d cases before the excuse, want the one seeded", len(before))
	}

	if _, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeExemptWithReason,
		Note:  "Art. 14(5)(c): the disclosure is laid down by the law we obtained them under",
		Now:   time.Now(),
	}); err != nil {
		t.Fatalf("excusing the duty: %v", err)
	}

	after, _, err := e.store.ListNoticeCases(ctx, nil, 0, "")
	if err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("the queue still holds %d cases after the excuse, want none: an excused duty "+
			"is discharged, and a queue that keeps showing it prompts work nobody owes", len(after))
	}

	// It is still readable by name, which is what an auditor asks for.
	closed, _, err := e.store.ListNoticeCases(ctx, []NoticeState{NoticeExemptWithReason}, 0, "")
	if err != nil {
		t.Fatalf("reading the excused cases: %v", err)
	}
	if len(closed) != 1 || closed[0].ID != id {
		t.Errorf("asking for excused cases answered %d rows, want the one that was excused", len(closed))
	}
}

// TestTheQueueRefusesAStateThatDoesNotExist. A caller naming a typo would
// otherwise read an empty page and conclude there are no duties, which is the
// one wrong answer a compliance queue must not give quietly.
func TestTheQueueRefusesAStateThatDoesNotExist(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)

	var verr *ValidationError
	_, _, err := e.store.ListNoticeCases(ctx, []NoticeState{"overdue"}, 0, "")
	if !errors.As(err, &verr) || verr.Field != fieldState {
		t.Fatalf("asking for a state that does not exist answered %v, want a validation error on %q",
			err, fieldState)
	}
}

// TestTheNoticeQueueIsGatedOnThePrivacyObject. A seat holding `contact` alone
// is the ordinary rep, and these rows say how a named contact was obtained and
// whether anybody has told them yet.
func TestTheNoticeQueueIsGatedOnThePrivacyObject(t *testing.T) {
	e := setupChannelConsent(t)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	if _, _, err := e.store.ListNoticeCases(e.ctx, nil, 0, ""); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep reading the notice queue got %v, want permission denied", err)
	}
	if _, err := e.store.AssignNoticeCase(e.ctx, id, e.user); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep taking a duty got %v, want permission denied", err)
	}
	if _, err := e.store.ExcuseNoticeCase(e.ctx, id, ExcuseInput{
		State: NoticeExemptWithReason, Note: "because I said so", Now: time.Now(),
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep excusing a duty got %v, want permission denied", err)
	}
}

// TestExcusingRefusesAStateThatIsNotAnExcuse. `completed` means a disclosure
// this installation sent was delivered, and letting this path write it would
// let an officer certify a mail that never existed.
func TestExcusingRefusesAStateThatIsNotAnExcuse(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	for _, state := range []NoticeState{NoticeCompleted, NoticeOpen, NoticeNotRequired, NoticeQueued} {
		var verr *ValidationError
		_, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
			State: state, Note: "a ground", Now: time.Now(),
		})
		if !errors.As(err, &verr) || verr.Field != fieldState {
			t.Errorf("excusing into %q answered %v, want a validation error on %q", state, err, fieldState)
		}
	}
}

// TestABlockedDutyCanStillBeExcused is the case the obstacle makes most likely,
// and the one a naive write gets wrong.
//
// Blocked says the product cannot discharge this duty by sending anything —
// which is exactly when an officer reaches for "the subject already has the
// information" or "notice is impossible". The table's blocked_shape CHECK holds
// that a reason is present exactly when the state is blocked, so an excuse that
// leaves the obstacle behind is refused by the database as a raw constraint
// violation, and the officer reads a fault rather than a refusal.
func TestABlockedDutyCanStillBeExcused(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})
	if _, err := e.owner.Exec(context.Background(), `
		UPDATE privacy_notice_case
		   SET state = 'blocked', blocked_reason = 'no address on file'
		 WHERE id = $1`, id); err != nil {
		t.Fatalf("blocking the case: %v", err)
	}

	got, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{
		State: NoticeExemptWithReason,
		Note:  "Art. 14(5)(b): no address was ever held, notice is impossible",
		Now:   time.Now(),
	})
	if err != nil {
		t.Fatalf("excusing a blocked duty: %v — the obstacle is the reason an officer "+
			"reaches for an exemption, so this is the path that matters most", err)
	}
	if got.State != NoticeExemptWithReason {
		t.Errorf("the excused case is %q, want %q", got.State, NoticeExemptWithReason)
	}
	if got.BlockedReason != nil {
		t.Errorf("the excused case still names obstacle %q: the duty has ended, and a row "+
			"asserting both that it is exempt and that something blocks it is a contradiction "+
			"the blocked_shape CHECK refuses", *got.BlockedReason)
	}
}

// TestAReassignmentRecordsWhoHadItBefore. The state does not move on a
// reassignment — it is `assigned` before and after — so an audit image of the
// state alone would record "assigned to assigned" and lose the only fact the
// write actually carries.
func TestAReassignmentRecordsWhoHadItBefore(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	var second ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO app_user (id, email, display_name)
		VALUES (gen_random_uuid(), $1, 'Second officer') RETURNING id`,
		"second-"+e.contact.String()+"@cc.test").Scan(&second); err != nil {
		t.Fatalf("seeding the second seat: %v", err)
	}

	if _, err := e.store.AssignNoticeCase(ctx, id, e.user); err != nil {
		t.Fatalf("the first assignment: %v", err)
	}
	if _, err := e.store.AssignNoticeCase(ctx, id, second); err != nil {
		t.Fatalf("the reassignment: %v", err)
	}

	var before map[string]any
	if err := e.owner.QueryRow(context.Background(), `
		SELECT before FROM audit_log
		 WHERE entity_type = 'privacy_notice_case' AND entity_id = $1
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`, id).Scan(&before); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if before[fieldOwner] != e.user.String() {
		t.Errorf("the reassignment's before-image names owner %v, want the seat it was taken "+
			"from (%v): the state does not move on a reassignment, so the owner is the only "+
			"fact the image has to carry", before[fieldOwner], e.user)
	}
}

// TestOneDutyReadsTheSameAsItDoesInTheQueue. The detail read exists for a
// surface that opens one case, and it has to agree with the list about what a
// case is — which is why both go through noticeCaseColumns.
func TestOneDutyReadsTheSameAsItDoesInTheQueue(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})
	if _, err := e.store.AssignNoticeCase(ctx, id, e.user); err != nil {
		t.Fatalf("taking the duty: %v", err)
	}

	one, err := e.store.GetNoticeCase(ctx, id)
	if err != nil {
		t.Fatalf("reading the duty: %v", err)
	}
	listed, _, err := e.store.ListNoticeCases(ctx, nil, 0, "")
	if err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("the queue holds %d cases, want the one seeded", len(listed))
	}
	if !reflect.DeepEqual(one, listed[0]) {
		t.Errorf("the detail read answers %+v and the queue answers %+v: a surface opening "+
			"one duty and a surface listing them must not disagree about what a case is",
			one, listed[0])
	}
}

// TestADutyThatDoesNotExistIsNotFound, rather than an empty case that reads as
// one with no deadline.
func TestADutyThatDoesNotExistIsNotFound(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)

	if _, err := e.store.GetNoticeCase(ctx, ids.NewV7()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading a duty that does not exist answered %v, want not found", err)
	}
}

// TestTheDetailReadIsGatedLikeTheQueue. A seat that may not see the list may
// not see one of its rows either — a narrower detail read would be a different
// answer to the same question.
func TestTheDetailReadIsGatedLikeTheQueue(t *testing.T) {
	e := setupChannelConsent(t)
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})

	if _, err := e.store.GetNoticeCase(e.ctx, id); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep reading one duty got %v, want permission denied", err)
	}
}

// seedNoticeCaseDue seeds one duty at a stated deadline, so a case about
// ORDER can pin the order rather than hope two `now()` calls land apart.
func seedNoticeCaseDue(t *testing.T, e *channelConsentEnv, due time.Time) ids.UUID {
	t.Helper()
	acq := seedAcquisition(t, e, "purchased_or_imported")
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO privacy_notice_case
		       (contact_id, acquisition_id, rule, due_at, allowed_routes, state)
		VALUES ($1, $2, 'art14', $3, ARRAY['record_confirmation'], 'open')
		RETURNING id`, e.contact, acq, due).Scan(&id); err != nil {
		t.Fatalf("seeding the notice case: %v", err)
	}
	return id
}

// walkQueue reads the whole queue a page at a time, the way a caller following
// the contract does, and answers what it saw.
//
// It refuses to loop forever rather than trusting the cursor to advance: a
// walk that never terminates is how a paging bug arrives as a hung lane rather
// than a failing assertion.
func walkQueue(ctx context.Context, t *testing.T, e *channelConsentEnv, perPage int) []ids.UUID {
	t.Helper()
	var seen []ids.UUID
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("the queue walk did not finish in ten pages — the cursor is not advancing")
		}
		page, info, err := e.store.ListNoticeCases(ctx, nil, perPage, cursor)
		if err != nil {
			t.Fatalf("reading page %d of the queue: %v", pages, err)
		}
		for _, c := range page {
			seen = append(seen, c.ID)
		}
		if !info.HasMore {
			return seen
		}
		if info.NextCursor == "" {
			t.Fatal("the queue says there is another page and hands back no cursor to fetch it with")
		}
		cursor = info.NextCursor
	}
}

// TestEveryDutyIsReachableHoweverManyThereAre.
//
// The queue was a single bounded read with no continuation, so a duty ranked
// past the limit was absent from every answer the route could give — not slow
// to reach, not on page two, structurally unreachable, with the screen giving
// no sign a tail existed. A privacy officer working the queue in good faith
// would believe they had seen every duty the installation owes.
func TestEveryDutyIsReachableHoweverManyThereAre(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)

	base := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	want := make([]ids.UUID, 0, 5)
	for i := range 5 {
		want = append(want, seedNoticeCaseDue(t, e, base.Add(time.Duration(i)*time.Hour)))
	}

	got := walkQueue(ctx, t, e, 2)
	if len(got) != len(want) {
		t.Fatalf("walking the queue two at a time saw %d duties, want %d — a duty past the "+
			"first page is one the installation owes and nobody can see", len(got), len(want))
	}
	for i, id := range want {
		if got[i] != id {
			t.Errorf("duty %d of the walk is %s, want %s: the pages are not in deadline order, "+
				"so the officer working soonest-first is not", i, got[i], id)
		}
	}
}

// TestTwoDutiesFallingDueTogetherAreBothReached.
//
// The tie-break is the half an id-keyed cursor would lose. The queue orders by
// (due_at, id), and a walk resumed on id alone skips every case whose deadline
// is later and whose id happens to be smaller — silently, and only on an
// installation where two duties share a second, which is ordinary once a
// nightly import files a batch of them.
func TestTwoDutiesFallingDueTogetherAreBothReached(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)

	together := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	sharing := []ids.UUID{
		seedNoticeCaseDue(t, e, together),
		seedNoticeCaseDue(t, e, together),
	}
	// The queue's own tie-break, applied here so the expectation is the
	// ordering rather than the order the ids happened to be minted in.
	slices.SortFunc(sharing, func(a, b ids.UUID) int {
		return strings.Compare(a.String(), b.String())
	})
	want := append(slices.Clone(sharing), seedNoticeCaseDue(t, e, together.Add(time.Hour)))

	// The SEQUENCE, not the set. Presence alone is not the property: an
	// id-keyed cursor can still reach every row while serving one of them
	// twice and another out of deadline order, which is the same officer
	// reading the same duty on two pages and trusting the order of neither.
	got := walkQueue(ctx, t, e, 1)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("walking one at a time saw %v, want %v — two duties sharing a deadline are "+
			"separated by the id tie-break, and a cursor carrying only one half of the key "+
			"loses that", got, want)
	}
}

// TestACursorTheQueueDidNotMintIsRefused. An empty page would tell a privacy
// officer there is nothing left to do, which is the one answer this route must
// never guess at.
func TestACursorTheQueueDidNotMintIsRefused(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	seedNoticeCaseDue(t, e, time.Now().Add(time.Hour))

	var malformed *storekit.MalformedCursorError
	_, _, err := e.store.ListNoticeCases(ctx, nil, 0, "not-a-cursor")
	if !errors.As(err, &malformed) {
		t.Fatalf("a cursor this queue did not mint answered %v, want a malformed-cursor refusal", err)
	}
}

// TestTheQueuesWireCarriesItsPage drives the HANDLER, not the store.
//
// The store can page correctly and the route still answer a body with no
// continuation in it — the envelope is assembled here, and a client walks what
// the wire says rather than what the store returned. So this asserts the shape
// the contract declares: `data` and `page`, with a cursor that is accepted back
// on the next request.
func TestTheQueuesWireCarriesItsPage(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	base := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	for i := range 3 {
		seedNoticeCaseDue(t, e, base.Add(time.Duration(i)*time.Hour))
	}
	h := Handlers{store: e.store}

	read := func(cursor *string) (data []crmcontracts.NoticeCase, page crmcontracts.PageInfo) {
		t.Helper()
		limit := 2
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/privacy/notice-cases", nil).WithContext(ctx)
		h.ListNoticeCases(rec, req, crmcontracts.ListNoticeCasesParams{Limit: &limit, Cursor: cursor})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Data []crmcontracts.NoticeCase `json:"data"`
			Page crmcontracts.PageInfo     `json:"page"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decoding the queue's answer: %v", err)
		}
		return body.Data, body.Page
	}

	first, page := read(nil)
	if len(first) != 2 || !page.HasMore {
		t.Fatalf("first page carried %d duties and has_more %v, want 2 and true", len(first), page.HasMore)
	}
	if page.NextCursor == nil || *page.NextCursor == "" {
		t.Fatal("the wire says there is another page and hands back no cursor to fetch it with")
	}

	second, page := read(page.NextCursor)
	if len(second) != 1 || page.HasMore {
		t.Fatalf("second page carried %d duties and has_more %v, want 1 and false", len(second), page.HasMore)
	}
	if page.NextCursor != nil {
		t.Errorf("the last page handed back cursor %q — a client walking until has_more is false is fine, but one walking until the cursor is null would loop", *page.NextCursor)
	}
	if second[0].Id == first[0].Id || second[0].Id == first[1].Id {
		t.Error("the second page repeated a duty from the first")
	}
}
