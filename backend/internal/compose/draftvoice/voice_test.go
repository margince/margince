// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftvoice

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftrules"
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
	got, _ := Load(ctx, failing, nil)
	if !got.Degraded {
		t.Fatal("a lookup error must mark the context Degraded, or the loss is invisible on the wire")
	}
	if got.OK {
		t.Fatal("a degraded context must not claim a loaded voice")
	}

	absent := readerFunc(func(context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error) {
		return ai.VoiceProfile{}, ai.VoiceProfileVersion{}, false, nil
	})
	if got, _ := Load(ctx, absent, nil); got.Degraded || got.OK {
		t.Fatal("no profile is the product working: neither OK nor Degraded")
	}

	if got, _ := Load(ctx, nil, nil); got.Degraded || got.OK {
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

	got, _ := Load(ctx, rulesReader{rules: "Never offer a discount in writing."}, nil)
	if got.OK || got.Degraded {
		t.Fatalf("a workspace with rules and a rep with no profile is neither voiced nor degraded: %+v", got)
	}
	preamble := got.Preamble(fence)
	if !strings.Contains(preamble, "Never offer a discount in writing.") || !strings.Contains(preamble, "Team rules:") {
		t.Errorf("the team rules are missing from the user turn: %q", preamble)
	}

	blank, _ := Load(ctx, rulesReader{rules: "  \n"}, nil)
	if empty := blank.Preamble(fence); empty != "" {
		t.Errorf("blank rules must add nothing to the user turn, got %q", empty)
	}

	voiced := Context{OK: true, TeamRules: "Rule one."}
	voiced.Version.VoiceProfileMD = "Short sentences."
	text := voiced.Preamble(fence)
	if strings.Index(text, "Rule one.") > strings.Index(text, "Short sentences.") {
		t.Errorf("the team rules must precede the voice profile: %q", text)
	}
}

// A failed rules read refuses to draft, wrapping the cause under the 503 the
// caller answers; a failed voice read only degrades.
func TestAFailedTeamRulesReadRefusesWhileAFailedVoiceReadOnlyDegrades(t *testing.T) {
	t.Parallel()
	cause := errors.New("connection refused")
	_, err := Load(context.Background(), rulesReader{err: cause}, nil)
	if !errors.Is(err, ErrRulesUnavailable) || !errors.Is(err, cause) {
		t.Fatalf("an unreadable team rules setting must refuse with ErrRulesUnavailable: %v", err)
	}
	if _, err := LoadTeamRules(context.Background(), rulesReader{err: cause}, nil); !errors.Is(err, ErrRulesUnavailable) {
		t.Fatalf("LoadTeamRules must refuse too: %v", err)
	}
	voiceDown := readerFunc(func(context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error) {
		return ai.VoiceProfile{}, ai.VoiceProfileVersion{}, false, cause
	})
	got, err := Load(context.Background(), voiceDown, nil)
	if err != nil || !got.Degraded {
		t.Fatalf("a failed voice read degrades the voice only: %+v %v", got, err)
	}
}

func TestTheTeamRulesParagraphIsInsideTheSharedRulesAndTheProfileRanksBelowIt(t *testing.T) {
	t.Parallel()
	if !strings.Contains(draftrules.Shared, draftrules.TeamRules) {
		t.Error("the shared rules do not carry the team rules paragraph, so four of the surfaces would not either")
	}
	if !strings.Contains(SystemRule, "the team rules win") {
		t.Error("a voiced system turn no longer ranks the profile's human-authored rules below the team rules")
	}
}
