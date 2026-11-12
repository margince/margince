// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Throttle brakes the public intake, each ceiling escalating from a 429 with
// a growing wait to a block that ends on its own. The narrowest ceiling counts
// first, and a wider one counts only a post every narrower one admitted, so
// one sender spends no one else's budget.
type Throttle struct {
	installation, workspace, form, formAddress, address *ratelimit.Escalator
	// unknownKeys bounds the lookups of keys memory cannot answer, over every
	// sender, so made-up keys cannot turn into unbounded database reads.
	unknownKeys *ratelimit.Limiter
	preflights  *ratelimit.Escalator
	known       *knownForms
	log         *slog.Logger
}

// ThrottleLevel names the ceiling that refused a post.
type ThrottleLevel string

const (
	LevelInstallation ThrottleLevel = "installation"
	LevelWorkspace    ThrottleLevel = "workspace"
	LevelForm         ThrottleLevel = "form"
	LevelFormAddress  ThrottleLevel = "form_address"
	LevelAddress      ThrottleLevel = "address"
	LevelPreflight    ThrottleLevel = "preflight"
)

// installationKey is the one key the installation-wide ceilings count under.
const installationKey = "all"

// NewThrottle builds the ceilings. Each refuses when its store cannot be read,
// because an unmetered flood lands rows that cannot be taken back. list loads
// every live form for memory; lookup reads one key memory cannot answer.
func NewThrottle(limits ThrottleLimits, now func() time.Time, log *slog.Logger, list FormLister, lookup FormLookup) *Throttle {
	esc := func(name string, perMinute, minSenders int) *ratelimit.Escalator {
		return ratelimit.NewEscalator(name, ratelimit.FailClosed, ratelimit.Escalation{
			PerMinute: perMinute, BlockAfter: limits.BlockAfter, Cooldown: limits.Cooldown,
			MaxCooldown: limits.MaxCooldown, MinSenders: minSenders,
		}, now)
	}
	return &Throttle{
		installation: esc("public-forms/installation", limits.PerInstallation, limits.MinSenders),
		workspace:    esc("public-forms/workspace", limits.PerWorkspace, limits.MinSenders),
		form:         esc("public-forms/per-form", limits.PerForm, 0),
		formAddress:  esc("public-forms/per-ip", limits.PerIP, 0),
		address:      esc("public-forms/per-address", limits.PerAddress, 0),
		unknownKeys:  ratelimit.NewWithClock("public-forms/unknown-keys", ratelimit.FailClosed, limits.UnknownKeys, time.Minute, now),
		preflights:   esc("public-forms/preflight-per-address", limits.Preflights, 0),
		known:        newKnownForms(lookup, list, now, limits.FormRefresh, limits.FormExpiry),
		log:          log,
	}
}

// ThrottleVerdict is Check's answer. Zero means the post may go on; Unknown
// means its key names no form, which the edge answers itself.
type ThrottleVerdict struct {
	Refused    bool
	Unknown    bool
	RetryAfter time.Duration
	Level      ThrottleLevel
}

type throttleStep struct {
	level ThrottleLevel
	esc   *ratelimit.Escalator
	key   string
}

func (t *Throttle) wide(workspace, formKey string) []throttleStep {
	return []throttleStep{
		{LevelInstallation, t.installation, installationKey},
		{LevelWorkspace, t.workspace, workspace},
		{LevelForm, t.form, formKey},
	}
}

// blocked reads the wide blocks without counting and before any database work.
func (t *Throttle) blocked(workspace, formKey string) (ThrottleVerdict, bool) {
	for _, step := range t.wide(workspace, formKey) {
		if until, refused := step.esc.BlockedUntil(step.key); refused {
			return ThrottleVerdict{Refused: true, RetryAfter: until.Sub(t.known.now()), Level: step.level}, true
		}
	}
	return ThrottleVerdict{}, false
}

// Check counts one post to formKey from address in workspace.
//
// A block of the form, workspace or installation refuses first. Then the
// sender's own ceiling counts every post. A key that names no form counts
// nowhere wider. The rest count narrow to wide.
func (t *Throttle) Check(ctx context.Context, workspace, formKey, address string) ThrottleVerdict {
	if v, refused := t.blocked(workspace, formKey); refused {
		return v
	}
	if v := t.count(throttleStep{LevelAddress, t.address, address}, formKey, address); v.Refused {
		return v
	}
	if _, v := t.resolve(ctx, formKey); v.Refused || v.Unknown {
		return v
	}
	wide := t.wide(workspace, formKey)
	for _, step := range []throttleStep{{LevelFormAddress, t.formAddress, formKey + "|" + address}, wide[2], wide[1], wide[0]} {
		if v := t.count(step, formKey, address); v.Refused {
			if step.level == LevelFormAddress {
				t.form.NoteRefusal(formKey)
			}
			return v
		}
	}
	return ThrottleVerdict{}
}

// Preflight meters a CORS preflight on its own per-address ceiling, so it
// spends no form's budget, and answers the form whose origins it may allow.
func (t *Throttle) Preflight(ctx context.Context, workspace, formKey, address string) (Form, ThrottleVerdict) {
	if v, refused := t.blocked(workspace, formKey); refused {
		return Form{}, v
	}
	if v := t.count(throttleStep{LevelPreflight, t.preflights, address}, formKey, address); v.Refused {
		return Form{}, v
	}
	return t.resolve(ctx, formKey)
}

// resolve reads the form formKey names, from memory first. A key memory cannot
// answer is looked up only while the budget for such lookups lasts. Past it the
// key is answered as one that names no form, never refused, so a flood of
// made-up keys cannot stop a real form.
func (t *Throttle) resolve(ctx context.Context, formKey string) (Form, ThrottleVerdict) {
	if f, known, kept := t.known.kept(ctx, formKey); kept {
		return f, ThrottleVerdict{Unknown: !known}
	}
	if t.unknownKeys.Blocked(installationKey) {
		return Form{}, ThrottleVerdict{Unknown: true}
	}
	t.unknownKeys.Record(installationKey)
	f, known := t.known.read(ctx, formKey)
	return f, ThrottleVerdict{Unknown: !known}
}

// FormChanged forgets formKey and loads every form again on the next post. A
// form created, changed or archived on this replica is then seen at once.
func (t *Throttle) FormChanged(formKey string) { t.known.changed(formKey) }

func (t *Throttle) count(step throttleStep, formKey, address string) ThrottleVerdict {
	v := step.esc.Check(step.key, address)
	if !v.Refused {
		return ThrottleVerdict{}
	}
	t.logChange(step.level, formKey, v)
	return ThrottleVerdict{Refused: true, RetryAfter: v.RetryAfter, Level: step.level}
}

func (t *Throttle) logChange(level ThrottleLevel, formKey string, v ratelimit.Verdict) {
	if v.Change == ratelimit.NoChange {
		return
	}
	if level != LevelForm && level != LevelFormAddress {
		formKey = ""
	}
	if level == LevelInstallation {
		t.log.Error("web form intake is refusing posts to every form: too much traffic",
			"level", level, "step", v.Change, "until", v.BlockedUntil)
		return
	}
	t.log.Warn("web form intake is refusing posts: too many submissions",
		"level", level, "step", v.Change, "form_key", formKey, "until", v.BlockedUntil)
}

// IntakeState is what a form's admin is told about its intake now. A zero
// State means posts are taken as usual.
type IntakeState struct {
	State string
	Since *time.Time
	Until *time.Time
}

// The intake states a form shows.
const (
	IntakeThrottled = "throttled"
	IntakeBlocked   = "blocked"
	IntakePausedAll = "paused_all"
)

// IntakeState answers formKey's state, the widest ceiling first: a block of the
// whole installation or workspace pauses every form.
func (t *Throttle) IntakeState(ctx context.Context, formKey string) IntakeState {
	workspace := ""
	if ws, ok := principal.WorkspaceID(ctx); ok {
		workspace = ws.String()
	}
	for _, level := range []struct {
		esc *ratelimit.Escalator
		key string
		all bool
	}{{t.installation, installationKey, true}, {t.workspace, workspace, true}, {t.form, formKey, false}} {
		since, until, err := level.esc.State(level.key)
		if err != nil {
			t.log.Warn("web form intake state could not be read", "err", err)
			return IntakeState{}
		}
		if state, ok := intakeState(since, until, level.all); ok {
			return state
		}
	}
	return IntakeState{}
}

func intakeState(since, until time.Time, all bool) (IntakeState, bool) {
	switch {
	case all && !until.IsZero():
		return IntakeState{State: IntakePausedAll, Until: &until}, true
	case !until.IsZero():
		return IntakeState{State: IntakeBlocked, Until: &until}, true
	case !since.IsZero():
		return IntakeState{State: IntakeThrottled, Since: &since}, true
	}
	return IntakeState{}, false
}
