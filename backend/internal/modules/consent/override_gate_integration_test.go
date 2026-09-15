// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A standing override flips a machine reading that CanBeOverruled, and
// nothing else: not a subject's own act, and not an absolute machine fact —
// both of which the gate refuses before liveOverride is ever consulted.
//
// Integration because the row lives in Postgres. Every override here is
// seeded by a direct INSERT rather than through Store.Allow (override.go): this
// file is about what liveOverride reads, not about who may write the row —
// override_integration_test.go covers the writer door itself, end to end.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// seedOverride records one live communication_override row directly, the
// writer door's eventual shape minus its own auth/audit machinery — this test
// is about what liveOverride reads, not about who may write the row.
//
// Always at user level: liveOverride treats every level above machine alike
// (levelsAboveMachine), so the read path this file exercises has no arm that
// distinguishes user from admin — that distinction belongs to
// override_integration_test.go, which drives the real writer.
func (e *resolveEnv) seedOverride(t *testing.T, category string, revoked bool) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	var revokedAt *time.Time
	if revoked {
		now := time.Now()
		revokedAt = &now
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_override
		    (id, contact_id, category, reason, decided_by_level, captured_by, revoked_at)
		VALUES ($1, $2, $3, 'the rep called them last week', 'user', 'human:x', $4)`,
		id, e.contact, category, revokedAt); err != nil {
		t.Fatalf("seeding the override: %v", err)
	}
	return id
}

// A live user-level override flips a no-evidence refusal for its category.
func TestAnOverrideFlipsANoEvidenceRefusal(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	req := commsauthz.Request{LegacyPurposeKey: "newsletter"}

	// BEFORE the override: no consent on file, so the engine cannot support a
	// marketing send on its own reading.
	before := e.decide(t, req)
	if before.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow before any override existed — the fixture proves nothing")
	}
	if before.ReasonCode != commsauthz.ReasonNoMarketingConsent {
		t.Fatalf("reason = %q, want %q: this test is about the no-evidence reading, not some other refusal",
			before.ReasonCode, commsauthz.ReasonNoMarketingConsent)
	}

	overrideID := e.seedOverride(t, "marketing", false)

	after := e.decide(t, req)
	if after.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("verdict = %q (%s), want allow: a live user-level override should have vouched for this send",
			after.Verdict, after.ReasonCode)
	}
	if after.ReasonCode != commsauthz.ReasonAllowedByOverride {
		t.Errorf("reason = %q, want %q", after.ReasonCode, commsauthz.ReasonAllowedByOverride)
	}
	if after.OverrideID != overrideID {
		t.Errorf("override id = %s, want %s: the decision must name the row that vouched for it",
			after.OverrideID, overrideID)
	}
}

// An override for one category does not flip a refusal in another.
func TestAnOverrideIsScopedToItsCategory(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	req := commsauthz.Request{LegacyPurposeKey: "newsletter"}

	// A live override, but for a category this send never resolves to.
	e.seedOverride(t, "invoice_or_payment", false)

	got := e.decide(t, req)
	if got.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow (%s): an override for invoice_or_payment must not vouch for a "+
			"marketing send", got.ReasonCode)
	}
	if got.ReasonCode != commsauthz.ReasonNoMarketingConsent {
		t.Errorf("reason = %q, want %q: the marketing refusal must stand, untouched",
			got.ReasonCode, commsauthz.ReasonNoMarketingConsent)
	}
}

// An override never touches a subject act: a subject_request suppression
// binds marketing (it is not one of the three categories Art. 18(2) spares),
// and CanBeOverruled refuses it before liveOverride is ever asked — a live
// override for the same contact and category must not lift it.
func TestAnOverrideCannotFlipASubjectStop(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	e.suppress(t, "subject_request")
	e.seedOverride(t, "marketing", false)

	got := e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})
	if got.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny: they asked us to stop, and no seat's override may undo that",
			got.Verdict)
	}
	if got.ReasonCode != commsauthz.ReasonSubjectRequest {
		t.Errorf("reason = %q, want %q: the subject's own act, not the override's business",
			got.ReasonCode, commsauthz.ReasonSubjectRequest)
	}
	if got.OverrideID != (ids.UUID{}) {
		t.Errorf("override id = %s, want the zero value: nothing vouched for this send", got.OverrideID)
	}
}

// An override never touches an absolute machine fact: a hard bounce takes the
// early exit in decideOne before resolution, applySuppression or liveOverride
// ever run, so a live override for the same contact and category cannot reach
// it.
func TestAnOverrideCannotFlipAHardBounce(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	e.suppress(t, "hard_bounce")
	e.seedOverride(t, "marketing", false)

	got := e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})
	if got.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny: no template makes a dead address accept mail", got.Verdict)
	}
	if got.ReasonCode != commsauthz.ReasonHardBounce {
		t.Errorf("reason = %q, want %q", got.ReasonCode, commsauthz.ReasonHardBounce)
	}
}

// An override never answers an unknown-purpose refusal. A send naming a purpose
// key nothing defines resolves to no category, so Resolved stays at its default
// (marketing) — a per-category vouch has nothing to answer, and a live marketing
// override for the same contact must NOT flip it. CanBeOverruledByCategory
// excludes it before liveOverride is ever consulted.
func TestAnOverrideCannotFlipAnUnknownPurpose(t *testing.T) {
	e := setupResolve(t)
	// No seedPurpose: the key is undefined, so the engine cannot resolve what
	// this send is and refuses with unknown_purpose.
	e.seedOverride(t, "marketing", false)

	got := e.decide(t, commsauthz.Request{LegacyPurposeKey: "not-a-real-purpose"})
	if got.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny: an unknown-purpose send resolves to no category, so a "+
			"marketing vouch has nothing to answer", got.Verdict)
	}
	if got.ReasonCode != commsauthz.ReasonUnknownPurpose {
		t.Errorf("reason = %q, want %q: the unknown-purpose refusal must stand",
			got.ReasonCode, commsauthz.ReasonUnknownPurpose)
	}
	if got.OverrideID != (ids.UUID{}) {
		t.Errorf("override id = %s, want the zero value: nothing vouched for this send", got.OverrideID)
	}
}

// A revoked override does nothing: liveOverride's own WHERE clause excludes
// it, so the refusal it would have lifted stands.
func TestARevokedOverrideDoesNotApply(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	e.seedOverride(t, "marketing", true)

	got := e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})
	if got.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow (%s): a revoked override must not vouch for anything", got.ReasonCode)
	}
	if got.ReasonCode != commsauthz.ReasonNoMarketingConsent {
		t.Errorf("reason = %q, want %q: the original refusal must stand",
			got.ReasonCode, commsauthz.ReasonNoMarketingConsent)
	}
}
