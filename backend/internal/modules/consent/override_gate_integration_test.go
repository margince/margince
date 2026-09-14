// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A standing override flips a machine reading that CanBeOverruled, and
// nothing else: not a subject's own act, and not an absolute machine fact —
// both of which the gate refuses before liveOverride is ever consulted.
//
// Integration because the row lives in Postgres and the writer door does not
// exist yet (Task 4): every override here is seeded by a direct INSERT, the
// shape a real writer will produce.

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
func (e *resolveEnv) seedOverride(t *testing.T, category, level string, revoked bool) ids.UUID {
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
		VALUES ($1, $2, $3, 'the rep called them last week', $4, 'human:x', $5)`,
		id, e.contact, category, level, revokedAt); err != nil {
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

	overrideID := e.seedOverride(t, "marketing", "user", false)

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
	e.seedOverride(t, "invoice_or_payment", "user", false)

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
	e.seedOverride(t, "marketing", "user", false)

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
	e.seedOverride(t, "marketing", "user", false)

	got := e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})
	if got.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny: no template makes a dead address accept mail", got.Verdict)
	}
	if got.ReasonCode != commsauthz.ReasonHardBounce {
		t.Errorf("reason = %q, want %q", got.ReasonCode, commsauthz.ReasonHardBounce)
	}
}

// A revoked override does nothing: liveOverride's own WHERE clause excludes
// it, so the refusal it would have lifted stands.
func TestARevokedOverrideDoesNotApply(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	e.seedOverride(t, "marketing", "user", true)

	got := e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})
	if got.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow (%s): a revoked override must not vouch for anything", got.ReasonCode)
	}
	if got.ReasonCode != commsauthz.ReasonNoMarketingConsent {
		t.Errorf("reason = %q, want %q: the original refusal must stand",
			got.ReasonCode, commsauthz.ReasonNoMarketingConsent)
	}
}
