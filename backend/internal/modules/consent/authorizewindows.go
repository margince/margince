// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// How long a piece of evidence keeps supporting a message.
//
// Every window here bounds an UNPROMPTED follow-up and none of them bounds a
// same-thread reply. The subject wrote to us and did not withdraw, so a rep
// answering a months-old thread is doing the ordinary thing — a window that
// refused that would be restricting correspondence rather than advertising,
// which is the failure mode that makes reps route around the product.
//
// A jurisdiction pack may shorten any of these; the packs that ship declare
// exactly these numbers so a reader can see they agree rather than inheriting
// silently. What a pack must not do is LENGTHEN one: the fold takes the shorter
// of the two (messagingrules.shorterWindow), so a pack naming a longer window
// than the core default is simply ignored.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// defaultReplyWindow is how long an inbound message keeps making an
	// UNPROMPTED follow-up lawful when there is no thread to continue.
	//
	// Twelve months. Hours because the type is a Duration and a month is not
	// one; the comparison is against a recorded timestamp, so the small drift
	// against calendar months falls on the permissive side of a bound that
	// governs a follow-up rather than a refusal.
	defaultReplyWindow = 365 * 24 * time.Hour

	// defaultDealFollowUpWindow is how long a live opportunity keeps
	// supporting an unprompted follow-up after the last interaction. Six
	// months, on the same reading as above.
	defaultDealFollowUpWindow = 182 * 24 * time.Hour
)

// windows are the spans an evidence check measures against.
type windows struct {
	reply      time.Duration
	dealFollow time.Duration
}

// windowsFor resolves the spans that bind this installation.
//
// A jurisdiction that declares a window shortens the core default; one that
// declares none, and an installation that names no country at all, gets the
// defaults above. That direction is deliberate — an installation with no
// country stated should behave exactly as it did before jurisdiction packs
// existed, rather than losing the ability to follow up because nobody has
// filled in a setting.
func (s *Store) windowsFor(ctx context.Context, tx pgx.Tx) (windows, error) {
	out := windows{reply: defaultReplyWindow, dealFollow: defaultDealFollowUpWindow}
	rules, applicable, err := s.applicableRules(ctx, tx)
	if err != nil {
		return windows{}, err
	}
	if !applicable {
		return out, nil
	}
	// Zero means the pack says nothing about this window, which is not the same
	// as a pack asking for zero — Rules.Validate refuses a negative one and the
	// published contract documents zero as "the core default".
	if rules.ReplyWindow > 0 && rules.ReplyWindow < out.reply {
		out.reply = rules.ReplyWindow
	}
	if rules.DealFollowUpWindow > 0 && rules.DealFollowUpWindow < out.dealFollow {
		out.dealFollow = rules.DealFollowUpWindow
	}
	return out, nil
}

// Why installation.country is MachineryApplied, recorded here because this is
// the reader that needs it:
//
// windowsFor runs inside the transaction that binds a decision, under whatever
// principal is sending. A rep holds no settings-read grant and a dispatching
// worker holds the system principal, so a gated read fails the send outright —
// with a 500, not a refusal, because "the caller may not read a setting" is not
// an answer about anybody's consent.
//
// That is not hypothetical: the entry shipped gated and killed every outbound
// send job on main until it was declared. TestEverySettingReadThroughApplyIsDeclaredMachineryApplied
// (backend/gates/machineryapplied_test.go) now derives the pairing from the
// source, so a reader added here without the declaration fails at build rather
// than in a worker log.
