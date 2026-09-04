// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftvoice

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

type readerFunc func(ctx context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error)

func (f readerFunc) ActiveVoiceForActor(ctx context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error) {
	return f(ctx)
}

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
