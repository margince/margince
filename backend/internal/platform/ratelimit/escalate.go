// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"strconv"
	"time"
)

// Escalation is how a per-minute ceiling tightens on a key that keeps going
// past it. Each further refusal waits longer, and then the key is blocked.
type Escalation struct {
	PerMinute int
	// BlockAfter is how many minutes in a row a key may go past PerMinute
	// before it is blocked.
	BlockAfter int
	// Cooldown is the first block's length. Each block within MaxCooldown of
	// the last one lasts twice as long, up to MaxCooldown.
	Cooldown    time.Duration
	MaxCooldown time.Duration
	// MinSenders is how many distinct senders a minute must hold before the
	// key is blocked. Below it the key only answers 429. Zero asks none.
	MinSenders int
}

// Escalator meters keys by the clock minute and escalates as Escalation says.
// It counts through a Limiter, so it shares that limiter's store, name scope
// and outage posture.
type Escalator struct {
	meter *Limiter
	esc   Escalation
}

// Change names the step a Check moved a key into, for the caller's log line.
type Change string

// The steps a Check can report. NoChange means the key stayed where it was.
const (
	NoChange       Change = ""
	StartedRefusal Change = "throttled"
	StartedBlock   Change = "blocked"
)

// Verdict is one Check's answer. RetryAfter is set on every refusal;
// BlockedUntil only while a block holds.
type Verdict struct {
	Refused      bool
	RetryAfter   time.Duration
	BlockedUntil time.Time
	Change       Change
}

// NewEscalator builds an escalator on this process's registry.
func NewEscalator(name string, kind Kind, esc Escalation, now func() time.Time) *Escalator {
	return process.NewEscalator(name, kind, esc, now)
}

// NewEscalator builds an escalator on this registry.
func (r *Registry) NewEscalator(name string, kind Kind, esc Escalation, now func() time.Time) *Escalator {
	return &Escalator{meter: r.NewWithClock(name, kind, esc.PerMinute, time.Minute, now), esc: esc}
}

// blindWait is what a refusal asks for when the store could not be read.
const blindWait = time.Minute

// Check counts one post for key from sender and answers whether it is refused.
// sender is read only when the escalation asks for MinSenders.
func (e *Escalator) Check(key, sender string) Verdict {
	if !meterable(key) || !meterable(sender) {
		return Verdict{}
	}
	now := e.meter.now()
	if until, err := e.held("block:"+key, now); err != nil {
		return e.blind(err)
	} else if until.After(now) {
		return Verdict{Refused: true, RetryAfter: until.Sub(now), BlockedUntil: until}
	}
	minute := now.Unix() / 60
	n, err := e.meter.store().count(e.meter.scoped("minute:"+key+":"+strconv.FormatInt(minute, 10)), 2*time.Minute, now)
	if err != nil {
		return e.blind(err)
	}
	if err := e.countSender(key, sender, minute, now); err != nil {
		return e.blind(err)
	}
	over := n - e.esc.PerMinute
	if over <= 0 {
		return Verdict{}
	}
	change := NoChange
	if over == 1 {
		if change, err = e.markMinuteOver(key, minute, now); err != nil {
			return e.blind(err)
		}
		if change == StartedBlock {
			until, err := e.held("block:"+key, now)
			if err != nil {
				return e.blind(err)
			}
			return Verdict{Refused: true, RetryAfter: until.Sub(now), BlockedUntil: until, Change: StartedBlock}
		}
	}
	return Verdict{Refused: true, RetryAfter: e.growingWait(over, minute, now), Change: change}
}

// growingWait is the time to the next minute, doubled for each further refused
// post this minute. It is never longer than a first block.
func (e *Escalator) growingWait(over int, minute int64, now time.Time) time.Duration {
	wait := time.Unix((minute+1)*60, 0).Sub(now)
	for range min(over-1, 20) {
		wait *= 2
		if wait >= e.esc.Cooldown {
			return e.esc.Cooldown
		}
	}
	return max(wait, time.Second)
}

// markMinuteOver records that key went past its ceiling this minute and keeps
// the time its refusals began. It blocks key once BlockAfter minutes in a row did.
func (e *Escalator) markMinuteOver(key string, minute int64, now time.Time) (Change, error) {
	s := e.meter.store()
	span := time.Duration(e.esc.BlockAfter+1) * time.Minute
	if err := s.hold(e.meter.scoped(overKey(key, minute)), 1, span, now); err != nil {
		return NoChange, err
	}
	change, err := e.recordRefusal(key, now)
	if err != nil {
		return NoChange, err
	}
	if enough, err := e.enoughSenders(key, minute, now); err != nil || !enough {
		return change, err
	}
	for back := int64(1); back < int64(e.esc.BlockAfter); back++ {
		v, err := s.held(e.meter.scoped(overKey(key, minute-back)), now)
		if err != nil || v == 0 {
			return change, err
		}
	}
	repeats, err := s.count(e.meter.scoped("blocks:"+key), 2*e.esc.MaxCooldown, now)
	if err != nil {
		return NoChange, err
	}
	cooldown := e.esc.Cooldown
	for range min(repeats-1, 20) {
		cooldown = min(cooldown*2, e.esc.MaxCooldown)
	}
	until := now.Add(cooldown)
	return StartedBlock, s.hold(e.meter.scoped("block:"+key), until.UnixMilli(), cooldown, now)
}

// countSender counts sender once a minute under key, when the escalation
// asks how many distinct senders a block needs.
func (e *Escalator) countSender(key, sender string, minute int64, now time.Time) error {
	if e.esc.MinSenders == 0 {
		return nil
	}
	s := e.meter.store()
	marker := e.meter.scoped("sender:" + key + ":" + strconv.FormatInt(minute, 10) + ":" + sender)
	seen, err := s.held(marker, now)
	if err != nil || seen > 0 {
		return err
	}
	if err := s.hold(marker, 1, 2*time.Minute, now); err != nil {
		return err
	}
	_, err = s.count(e.meter.scoped(sendersKey(key, minute)), 2*time.Minute, now)
	return err
}

func (e *Escalator) enoughSenders(key string, minute int64, now time.Time) (bool, error) {
	if e.esc.MinSenders == 0 {
		return true, nil
	}
	n, err := e.meter.store().peek(e.meter.scoped(sendersKey(key, minute)), 2*time.Minute, now)
	return n >= e.esc.MinSenders, err
}

func sendersKey(key string, minute int64) string {
	return "senders:" + key + ":" + strconv.FormatInt(minute, 10)
}

// BlockedUntil reads key's block without counting a post. A store it cannot
// read answers as Check would: refused for a fail-closed escalator.
func (e *Escalator) BlockedUntil(key string) (time.Time, bool) {
	if !meterable(key) {
		return time.Time{}, false
	}
	now := e.meter.now()
	until, err := e.held("block:"+key, now)
	if err != nil {
		v := e.blind(err)
		return now.Add(v.RetryAfter), v.Refused
	}
	return until, until.After(now)
}

func overKey(key string, minute int64) string {
	return "over:" + key + ":" + strconv.FormatInt(minute, 10)
}

// sinceSpan is how long "refused since" outlives the last minute that went over.
const sinceSpan = 2 * time.Minute

// NoteRefusal marks key as refused now without counting a post, for a caller
// that rolls a narrower key's refusal up into this one's state.
func (e *Escalator) NoteRefusal(key string) {
	if !meterable(key) {
		return
	}
	if _, err := e.recordRefusal(key, e.meter.now()); err != nil {
		e.meter.noteOutage(err, "note")
	}
}

func (e *Escalator) recordRefusal(key string, now time.Time) (Change, error) {
	s := e.meter.store()
	since, err := s.held(e.meter.scoped("since:"+key), now)
	if err != nil {
		return NoChange, err
	}
	change := NoChange
	if since == 0 {
		since, change = now.UnixMilli(), StartedRefusal
	}
	return change, s.hold(e.meter.scoped("since:"+key), since, sinceSpan, now)
}

// State answers when key's refusals began and until when it is blocked; a zero
// time means none.
func (e *Escalator) State(key string) (refusedSince, blockedUntil time.Time, err error) {
	now := e.meter.now()
	since, err := e.meter.store().held(e.meter.scoped("since:"+key), now)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if since > 0 {
		refusedSince = time.UnixMilli(since)
	}
	if blockedUntil, err = e.held("block:"+key, now); err != nil || !blockedUntil.After(now) {
		return refusedSince, time.Time{}, err
	}
	return refusedSince, blockedUntil, nil
}

func (e *Escalator) held(key string, now time.Time) (time.Time, error) {
	ms, err := e.meter.store().held(e.meter.scoped(key), now)
	if err != nil || ms == 0 {
		return time.Time{}, err
	}
	return time.UnixMilli(ms), nil
}

func (e *Escalator) blind(err error) Verdict {
	if e.meter.admitsBlind(err, "escalate") {
		return Verdict{}
	}
	return Verdict{Refused: true, RetryAfter: blindWait}
}
