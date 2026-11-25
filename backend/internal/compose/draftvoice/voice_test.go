// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftvoice

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

type readerFunc func(ctx context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error)

func (f readerFunc) ActiveVoiceForActor(ctx context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error) {
	return f(ctx)
}

func (readerFunc) TeamRules(context.Context) (string, error) { return "", nil }

// rulesReader answers the team rules the workspace saved, and no voice profile.
type rulesReader struct {
	rules string
	err   error
}

func (r rulesReader) ActiveVoiceForActor(context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error) {
	return ai.VoiceProfile{}, ai.VoiceProfileVersion{}, false, nil
}

func (r rulesReader) TeamRules(context.Context) (string, error) { return r.rules, r.err }

// A failed lookup and an absent profile are different answers, and the wire
// flag is the only way a sender learns about the first: the draft text reads
// the same either way.
func TestLoadTellsAFailedLookupFromAnAbsentProfile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	failing := readerFunc(func(context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error) {
		return ai.VoiceProfile{}, ai.VoiceProfileVersion{}, false, errors.New("connection refused")
	})
	got := Load(ctx, failing, nil)
	if !got.Degraded {
		t.Fatal("a lookup error must mark the context Degraded, or the loss is invisible on the wire")
	}
	if got.OK {
		t.Fatal("a degraded context must not claim a loaded voice")
	}

	absent := readerFunc(func(context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error) {
		return ai.VoiceProfile{}, ai.VoiceProfileVersion{}, false, nil
	})
	if got := Load(ctx, absent, nil); got.Degraded || got.OK {
		t.Fatal("no profile is the product working: neither OK nor Degraded")
	}

	if got := Load(ctx, nil, nil); got.Degraded || got.OK {
		t.Fatal("a deployment with no voice lane degrades nothing")
	}
}

// A rep with no voice profile still writes under the team rules. The rules come
// ahead of the voice block, so the user turn's order matches the system turn's
// ranking.
func TestTeamRulesReachTheUserTurnWithoutAVoiceProfile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fence := promptfence.New()

	got := Load(ctx, rulesReader{rules: "Never offer a discount in writing."}, nil)
	if got.OK || got.Degraded {
		t.Fatalf("a workspace with rules and a rep with no profile is neither voiced nor degraded: %+v", got)
	}
	preamble := got.Preamble(fence)
	if !strings.Contains(preamble, "Never offer a discount in writing.") || !strings.Contains(preamble, "Team rules:") {
		t.Errorf("the team rules are missing from the user turn: %q", preamble)
	}

	if empty := Load(ctx, rulesReader{rules: "  \n"}, nil).Preamble(fence); empty != "" {
		t.Errorf("blank rules must add nothing to the user turn, got %q", empty)
	}

	voiced := Context{OK: true, TeamRules: "Rule one."}
	voiced.Version.VoiceProfileMD = "Short sentences."
	text := voiced.Preamble(fence)
	if strings.Index(text, "Rule one.") > strings.Index(text, "Short sentences.") {
		t.Errorf("the team rules must precede the voice profile: %q", text)
	}
}

// A failed rules read drafts without them and says so on the wire, as a failed
// voice read does.
func TestAFailedTeamRulesReadDegradesTheContextButKeepsTheVoice(t *testing.T) {
	t.Parallel()
	got := Load(context.Background(), rulesReader{err: errors.New("connection refused")}, nil)
	if !got.Degraded || got.TeamRules != "" {
		t.Errorf("a failed rules read must mark the context Degraded with no rules: %+v", got)
	}
}
