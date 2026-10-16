// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The privacy module's settings declarations (ADR-0090/A135). Privacy supplies
// the type, default, validator, RBAC object and audit verb; platform/settings
// runs them without knowing what retention is.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// retentionPolicyObject is the RBAC object gating the whole retention surface —
// the policy rows AND this posture. Admin/ops-only on every verb, read
// included: whoever may author a rule about what the installation destroys is
// also who may suspend every such rule, and no other role has a consumer for
// either read.
const retentionPolicyObject = "retention_policy"

// RetainOnly is the installation's retain-only posture (GCS-PARAM-6): while it
// holds, the nightly evaluator applies no `anonymize` and no `erase`, whatever
// any policy row says. `archive` is untouched, because archiving retains.
//
// It is a POSTURE rather than an edit to the policy rows, and that is the whole
// design. An installation under a contractual keep-everything obligation keeps
// its ladder visible and intact — an admin can see exactly what would happen if
// the obligation lapsed — and lifting the posture resumes enforcement with
// nothing to re-author. Editing the rows away would have destroyed that
// information to express the same intent.
//
// Default FALSE, deliberately. Storage limitation (Art. 5(1)(e)) is what
// "compliant out of the box" means for the seeded ladder (DM-SEED-1..8), so the
// regulated installation opts IN rather than every installation opting out. A
// deployment that must never delete says so before first boot instead
// (`seeds.retention.default_policy: retain_only`, GCS-PARAM-7), which closes the
// window between seeding the rows and the first admin login.
//
// No validator: both values are legitimate postures, and one here would be
// ceremony. No freeze probe: a posture that could stop being changeable would
// be a trap — the obligation that justified it can lapse.
var RetainOnly = config.DefineStored(config.Spec[bool]{
	Key:     "privacy.retain_only",
	Env:     "MARGINCE_PRIVACY_RETAIN_ONLY",
	Type:    config.Bool,
	Default: config.Some(false),
	Doc:     "Destroy nothing: the nightly retention pass applies no anonymize and no erase, whatever a policy says.",
}, config.Store{RBACObject: retentionPolicyObject, AuditVerb: "update"})

// retainOnlyTx is the posture every retention decision reads, inside the
// caller's transaction, under the boot value the program bound.
func retainOnlyTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	return config.StoredValTx(ctx, tx, config.Bound(), RetainOnly)
}

// ConfigSettings are this module's settings in the configuration registry.
func ConfigSettings() []config.Any {
	return []config.Any{RetainOnly}
}

// Definitions is privacy's settings.Define entry: the carried floor, which a
// jurisdiction change writes and no operator sets.
func Definitions() []settings.Definition {
	return []settings.Definition{RetentionFloorCarry}
}
