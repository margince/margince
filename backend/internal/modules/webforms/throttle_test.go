// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type throttleClock struct{ t time.Time }

func (c *throttleClock) now() time.Time       { return c.t }
func (c *throttleClock) minute()              { c.t = c.t.Add(time.Minute) }
func (c *throttleClock) wait(d time.Duration) { c.t = c.t.Add(d) }

// throttleFixture starts on a minute boundary, so a first refusal waits a
// whole minute. A key that starts "unknown-" names no form.
type throttleFixture struct {
	*Throttle
	clock   *throttleClock
	lookups int
	// listed is what loading every live form answers, once listing is on.
	listed []Form
	loads  int
	// listFails makes every load fail; gone keys are absent to a lookup.
	listFails bool
	gone      map[string]bool
}

func newThrottleFixture(t *testing.T, limits ThrottleLimits) *throttleFixture {
	t.Helper()
	return newThrottleFixtureListing(t, limits, false)
}

// newThrottleFixtureListing loads every live form from f.listed when listing
// is on; otherwise each key is looked up alone.
func newThrottleFixtureListing(t *testing.T, limits ThrottleLimits, listing bool) *throttleFixture {
	t.Helper()
	f := &throttleFixture{clock: &throttleClock{t: time.Unix(28_333_334*60, 0)}, gone: map[string]bool{}}
	var list FormLister
	if listing {
		list = func(context.Context) ([]Form, error) {
			f.loads++
			if f.listFails {
				return nil, errors.New("the database is unavailable")
			}
			return f.listed, nil
		}
	}
	f.Throttle = NewThrottle(limits, f.clock.now, slog.New(slog.NewTextHandler(io.Discard, nil)), list,
		func(_ context.Context, key string) (Form, bool, bool) {
			f.lookups++
			if strings.HasPrefix(key, "unknown-") || f.gone[key] {
				return Form{}, false, true
			}
			return Form{Key: key, Sender: SenderBrowser, AllowedOrigins: []string{"https://www.example.com"}}, true, true
		})
	return f
}

func (f *throttleFixture) post(workspace, form, address string) ThrottleVerdict {
	return f.Check(context.Background(), workspace, form, address)
}

const (
	throttleWorkspace = "ws-1"
	throttleForm      = "form-key-0000000001"
)

func address(n int) string { return "198.51.100." + strconv.Itoa(n) }

func formKey(n int) string { return "form-key-" + strconv.Itoa(1_000_000_000+n) }

// openLimits leave only the ceilings a test names in force.
func openLimits() ThrottleLimits {
	l := DefaultThrottleLimits()
	l.PerAddress, l.PerIP, l.PerForm, l.PerWorkspace, l.PerInstallation = 1_000_000, 1_000_000, 1_000_000, 1_000_000, 1_000_000
	return l
}

func TestOneSenderFloodingAFormIsBlockedItselfWhileAnotherSendersPostLands(t *testing.T) {
	f := newThrottleFixture(t, DefaultThrottleLimits())
	for minute := range 5 {
		for i := range 5 {
			if v := f.post(throttleWorkspace, throttleForm, address(1)); v.Refused {
				t.Fatalf("minute %d post %d refused under the limit: %+v", minute, i+1, v)
			}
		}
		v := f.post(throttleWorkspace, throttleForm, address(1))
		if minute < 4 && (!v.Refused || v.Level != LevelFormAddress || v.RetryAfter != time.Minute) {
			t.Fatalf("minute %d sixth post = %+v, want a one-minute 429 for the sender", minute, v)
		}
		if minute == 0 {
			if again := f.post(throttleWorkspace, throttleForm, address(1)); again.RetryAfter != 2*time.Minute {
				t.Fatalf("a further post waits %s, want 2m", again.RetryAfter)
			}
		}
		if minute == 4 && (!v.Refused || v.Level != LevelFormAddress || v.RetryAfter != 15*time.Minute) {
			t.Fatalf("fifth minute over = %+v, want the sender blocked for 15 minutes", v)
		}
		f.clock.minute()
	}
	if v := f.post(throttleWorkspace, throttleForm, address(2)); v.Refused {
		t.Fatalf("another sender's post to the same form is refused: %+v", v)
	}
	if state := f.IntakeState(context.Background(), throttleForm); state.State != IntakeThrottled {
		t.Fatalf("the form shows %+v, want throttled while one sender is refused", state)
	}
	f.clock.wait(14 * time.Minute)
	if v := f.post(throttleWorkspace, throttleForm, address(1)); v.Refused {
		t.Fatalf("the sender's block did not end on its own: %+v", v)
	}
}

func TestOneSenderFloodingManyFormsSpendsNoWiderBudget(t *testing.T) {
	limits := DefaultThrottleLimits()
	limits.PerForm, limits.PerWorkspace, limits.PerInstallation = 15, 15, 15
	f := newThrottleFixture(t, limits)
	refused := 0
	for i := range 200 {
		if f.post(throttleWorkspace, formKey(i), address(1)).Refused {
			refused++
		}
	}
	if refused != 200-limits.PerAddress {
		t.Fatalf("the flooding sender was refused %d times, want every post past its own %d", refused, limits.PerAddress)
	}
	for _, sender := range []int{2, 3, 4} {
		if v := f.post(throttleWorkspace, formKey(sender), address(sender)); v.Refused {
			t.Fatalf("sender %d is refused in the minute one other sender flooded: %+v", sender, v)
		}
	}
}

func TestOneSenderNeverBlocksTheWorkspaceOrTheInstallation(t *testing.T) {
	limits := openLimits()
	limits.PerWorkspace, limits.PerInstallation = 20, 20
	f := newThrottleFixture(t, limits)
	var last ThrottleVerdict
	for range 10 {
		for i := range 30 {
			last = f.post(throttleWorkspace, formKey(i), address(1))
		}
		f.clock.minute()
	}
	if !last.Refused || last.Level != LevelWorkspace {
		t.Fatalf("one sender past the workspace limit = %+v, want a 429 from the workspace", last)
	}
	if _, blocked := f.workspace.BlockedUntil(throttleWorkspace); blocked {
		t.Fatal("one sender blocked the workspace")
	}
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	if state := f.IntakeState(ctx, formKey(0)); state.State == IntakePausedAll {
		t.Fatalf("one sender paused every form: %+v", state)
	}
	if v := f.post("ws-2", formKey(0), address(2)); v.Refused {
		t.Fatalf("one sender refused another workspace's post: %+v", v)
	}
}

func TestManySendersTogetherBlockTheWorkspaceThenTheInstallation(t *testing.T) {
	limits := DefaultThrottleLimits()
	f := newThrottleFixture(t, limits)
	next := 0
	flood := func(workspace string, posts int) ThrottleVerdict {
		var last ThrottleVerdict
		for range posts {
			next++
			last = f.post(workspace, formKey(next%50), address(next))
		}
		return last
	}
	for minute := range 5 {
		v := flood(throttleWorkspace, 121)
		if _, blocked := f.workspace.BlockedUntil(throttleWorkspace); minute < 4 && (!v.Refused || v.Level != LevelWorkspace || blocked) {
			t.Fatalf("minute %d = %+v, want a 429 from the workspace and no block yet", minute, v)
		}
		if minute == 4 && (!v.Refused || v.Level != LevelWorkspace || v.RetryAfter != limits.Cooldown) {
			t.Fatalf("fifth minute = %+v, want the workspace blocked", v)
		}
		f.clock.minute()
	}
	if v := f.post(throttleWorkspace, formKey(1), address(9999)); !v.Refused || v.Level != LevelWorkspace {
		t.Fatalf("a fresh sender inside the workspace block = %+v, want refused", v)
	}

	for minute := range 5 {
		var v ThrottleVerdict
		for w := range 6 {
			flood("ws-"+strconv.Itoa(minute*10+w+10), 100)
		}
		v = flood("ws-extra-"+strconv.Itoa(minute), 1)
		if minute == 4 && (!v.Refused || v.Level != LevelInstallation || v.RetryAfter != limits.Cooldown) {
			t.Fatalf("fifth minute = %+v, want the installation blocked", v)
		}
		if minute < 4 {
			f.clock.minute()
		}
	}
	state := f.IntakeState(principal.WithWorkspaceID(context.Background(), ids.NewV7()), "form-key-untouched01")
	if state.State != IntakePausedAll || state.Until == nil || !state.Until.Equal(f.clock.t.Add(limits.Cooldown)) {
		t.Fatalf("an untouched form shows %+v, want paused for all forms", state)
	}
	lookups := f.lookups
	if v := f.post("ws-fresh", "form-key-fresh000001", address(1)); !v.Refused || v.Level != LevelInstallation {
		t.Fatalf("a fresh form in a fresh workspace = %+v, want the installation's block", v)
	}
	if f.lookups != lookups {
		t.Fatal("a post the installation block refuses looked its form up")
	}
	f.clock.wait(limits.Cooldown)
	if v := f.post("ws-fresh", "form-key-fresh000001", address(1)); v.Refused {
		t.Fatalf("the installation block did not end on its own: %+v", v)
	}
}

func TestPostsToAKeyThatNamesNoFormCountTowardNoFormWorkspaceOrInstallation(t *testing.T) {
	limits := openLimits()
	limits.PerForm, limits.PerWorkspace, limits.PerInstallation = 5, 5, 5
	f := newThrottleFixture(t, limits)
	for i := range 500 {
		if v := f.post(throttleWorkspace, "unknown-key-000000001", address(i)); v.Refused || !v.Unknown {
			t.Fatalf("post %d to an unknown key = %+v, want it answered as unknown", i+1, v)
		}
	}
	for i := range 5 {
		if v := f.post(throttleWorkspace, throttleForm, address(1000+i)); v.Refused {
			t.Fatalf("post %d to a real form refused after posts to an unknown key: %+v", i+1, v)
		}
	}
	if f.lookups != 2 {
		t.Fatalf("the form keys were looked up %d times, want once each", f.lookups)
	}
}

func TestAQuietFormShowsNoIntakeState(t *testing.T) {
	f := newThrottleFixture(t, DefaultThrottleLimits())
	f.post(throttleWorkspace, throttleForm, address(1))
	if state := f.IntakeState(context.Background(), throttleForm); state.State != "" {
		t.Fatalf("a form under every limit shows %+v", state)
	}
}

// flood posts made-up keys from many senders until the budget for lookups of
// keys memory cannot answer is spent.
func (f *throttleFixture) flood(t *testing.T, posts int) {
	t.Helper()
	for i := range posts {
		if v := f.post(throttleWorkspace, "unknown-"+strconv.Itoa(1_000_000_000+i), address(i+1)); v.Refused || !v.Unknown {
			t.Fatalf("made-up key %d = %+v, want answered as unknown and never refused", i+1, v)
		}
	}
}

func TestAFloodOfMadeUpKeysFromManySendersReadsTheDatabaseABoundedNumberOfTimes(t *testing.T) {
	limits := DefaultThrottleLimits()
	f := newThrottleFixture(t, limits)
	f.post(throttleWorkspace, throttleForm, address(0))
	f.flood(t, 1000)
	if f.lookups != limits.UnknownKeys {
		t.Fatalf("a real form and 1000 made-up keys read the database %d times, want the budget's %d", f.lookups, limits.UnknownKeys)
	}
	if v := f.post(throttleWorkspace, throttleForm, address(5000)); v.Refused || v.Unknown {
		t.Fatalf("a real form's post after the flood = %+v, want it admitted", v)
	}
}

func TestARealFormNeverPostedToLandsAfterTheLookupBudgetIsSpent(t *testing.T) {
	f := newThrottleFixtureListing(t, DefaultThrottleLimits(), true)
	f.listed = []Form{{Key: throttleForm, Sender: SenderBrowser}}
	f.flood(t, 1000)
	if v := f.post(throttleWorkspace, throttleForm, address(5000)); v.Refused || v.Unknown {
		t.Fatalf("a real form's first post during the flood = %+v, want it admitted", v)
	}
}

func TestAFormCreatedDuringAFloodIsTakenWithinTheRefreshSpan(t *testing.T) {
	f := newThrottleFixtureListing(t, DefaultThrottleLimits(), true)
	f.flood(t, 1000)
	f.listed = []Form{{Key: "form-key-brandnew0001", Sender: SenderBrowser}}
	if v := f.post(throttleWorkspace, "form-key-brandnew0001", address(5000)); !v.Unknown {
		t.Fatalf("a form another replica just created = %+v, want unknown until the next load", v)
	}
	f.clock.wait(DefaultThrottleLimits().FormRefresh)
	if v := f.post(throttleWorkspace, "form-key-brandnew0001", address(5001)); v.Refused || v.Unknown {
		t.Fatalf("the new form after the refresh span = %+v, want it admitted", v)
	}
	f.listed = append(f.listed, Form{Key: "form-key-brandnew0002", Sender: SenderBrowser})
	f.FormChanged("form-key-brandnew0002")
	if v := f.post(throttleWorkspace, "form-key-brandnew0002", address(5002)); v.Refused || v.Unknown {
		t.Fatalf("a form created on this replica = %+v, want it admitted at once", v)
	}
}

func TestAPreflightFloodFromOneSenderIsRefusedWhileAnotherSendersPostLands(t *testing.T) {
	limits := DefaultThrottleLimits()
	f := newThrottleFixture(t, limits)
	ctx := context.Background()
	for i := range limits.Preflights {
		form, v := f.Preflight(ctx, throttleWorkspace, throttleForm, address(1))
		if v.Refused || !form.AllowsOrigin("https://www.example.com") {
			t.Fatalf("preflight %d = %+v, %+v; want answered with the form's origins", i+1, form, v)
		}
	}
	if _, v := f.Preflight(ctx, throttleWorkspace, throttleForm, address(1)); !v.Refused || v.Level != LevelPreflight {
		t.Fatalf("a preflight past the ceiling = %+v, want refused", v)
	}
	for i := range limits.PerIP {
		if v := f.post(throttleWorkspace, throttleForm, address(2)); v.Refused {
			t.Fatalf("another sender's post %d is refused by one sender's preflights: %+v", i+1, v)
		}
	}
	if v := f.post(throttleWorkspace, throttleForm, address(1)); v.Refused {
		t.Fatalf("the preflighting sender's own post spent its preflight budget: %+v", v)
	}
	if f.lookups != 1 {
		t.Fatalf("the preflights and posts read the form %d times, want once", f.lookups)
	}
}

func TestAPreflightDuringAFormBlockIsRefusedWithoutCounting(t *testing.T) {
	limits := DefaultThrottleLimits()
	f := newThrottleFixture(t, limits)
	next := 0
	for range 5 {
		for range limits.PerForm + 1 {
			next++
			f.post(throttleWorkspace, throttleForm, address(next))
		}
		f.clock.minute()
	}
	for range 3 * limits.Preflights {
		if _, v := f.Preflight(context.Background(), throttleWorkspace, throttleForm, address(1)); !v.Refused || v.Level != LevelForm {
			t.Fatalf("a preflight during the form's block = %+v, want refused by the form", v)
		}
	}
	f.clock.wait(limits.Cooldown)
	if _, v := f.Preflight(context.Background(), throttleWorkspace, throttleForm, address(1)); v.Refused {
		t.Fatalf("refused preflights spent the sender's budget: %+v", v)
	}
}

func TestOneSendersPostsToMadeUpKeysMeetItsOwnCeilingFirst(t *testing.T) {
	limits := DefaultThrottleLimits()
	f := newThrottleFixture(t, limits)
	for i := range limits.PerAddress {
		if v := f.post(throttleWorkspace, "unknown-"+strconv.Itoa(i), address(1)); v.Refused {
			t.Fatalf("made-up key %d refused under the sender's ceiling: %+v", i+1, v)
		}
	}
	if v := f.post(throttleWorkspace, "unknown-next", address(1)); !v.Refused || v.Level != LevelAddress {
		t.Fatalf("the sender's next made-up key = %+v, want refused for its address", v)
	}
	if f.lookups != limits.PerAddress {
		t.Fatalf("the refused post read the database: %d reads", f.lookups)
	}
}

func TestAnArchivedFormIsNoLongerKnownOnceItsEntryExpiresEvenWhileLoadsFail(t *testing.T) {
	limits := DefaultThrottleLimits()
	f := newThrottleFixtureListing(t, limits, true)
	f.listed = []Form{{Key: throttleForm, Sender: SenderBrowser}}
	if v := f.post(throttleWorkspace, throttleForm, address(1)); v.Unknown {
		t.Fatalf("a listed form = %+v, want it known", v)
	}
	f.gone[throttleForm], f.listFails = true, true
	f.clock.wait(limits.FormExpiry - time.Second)
	if v := f.post(throttleWorkspace, throttleForm, address(2)); v.Unknown {
		t.Fatalf("an unexpired entry = %+v, want it still served while loads fail", v)
	}
	f.clock.wait(time.Second)
	if v := f.post(throttleWorkspace, throttleForm, address(3)); !v.Unknown {
		t.Fatalf("an archived form after its entry expired = %+v, want it read again and unknown", v)
	}
}

func TestAFormArchivedThroughTheAppIsForgottenAtOnce(t *testing.T) {
	f := newThrottleFixtureListing(t, DefaultThrottleLimits(), true)
	f.listed = []Form{{Key: throttleForm, Sender: SenderBrowser}}
	f.post(throttleWorkspace, throttleForm, address(1))
	f.gone[throttleForm], f.listFails = true, true
	f.FormChanged(throttleForm)
	if v := f.post(throttleWorkspace, throttleForm, address(2)); !v.Unknown {
		t.Fatalf("a form archived through the app = %+v, want it unknown at once", v)
	}
}

func TestAFormKeptFromOneLookupIsReadAgainOnceItExpires(t *testing.T) {
	limits := DefaultThrottleLimits()
	f := newThrottleFixture(t, limits)
	f.post(throttleWorkspace, throttleForm, address(1))
	f.gone[throttleForm] = true
	f.clock.wait(limits.FormExpiry)
	if v := f.post(throttleWorkspace, throttleForm, address(2)); !v.Unknown {
		t.Fatalf("a form archived since its lookup = %+v, want it read again and unknown", v)
	}
}
