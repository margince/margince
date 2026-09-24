// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The retention-policy defaults (data-model §3.4): a new workspace is
// compliant out-of-the-box with conservative GDPR storage-limitation
// rows, editable per workspace. One action per row — the ladder is
// separate rows at increasing retain_days.

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// SeedDefaultRetentionTx plants the §3.4 default-of-record inside the
// workspace-bootstrap transaction, same C5 atomicity as the purpose
// catalog.
//
// Every row here is a DEFAULT an admin may edit, not a number this product
// fixes — embedCallRetention one file over is the fixed kind, and says so.
// Each lands `enabled`, so the retention engine acts on whatever number
// stands from the first sweep: what an admin edits is the window, never
// whether there is one. Changing a number HERE moves it only for
// installations bootstrapped afterwards, and an operator reading their own
// retention page is the one who decides for theirs.
//
// `raw_capture` is here because the table it governs is the largest thing this
// product stores — 92% of one measured database, 6.7 GB of 7.3 — and because
// until the provider original got a clock of its own it was reached only by the
// activity sweep's natural-key join and by Art. 17 erasure. An installation
// whose activity policy is the seeded 1095 days therefore kept every verbatim
// original for three years, and a scope shipping with no row at all meant that
// number was nobody's decision.
//
// 730 days rather than the activity's 1095: the original outlives nothing that
// reads it. The extracted activity survives this stage untouched — the selector
// joins to it precisely so it can — and with it the `(source_system, source_id)`
// tombstone a replay is refused by. What ages out is the second copy.
//
// It destroys less than the number suggests. The selector carries the statutory
// correspondence floor and the legal-hold test, so a Handelsbrief inside its
// window keeps its original however short this is set; and an original with no
// activity row is left alone, because there the raw_capture row IS the
// tombstone.
//
// `ai_call_payload` / `content` is the one worth reading twice. With payload
// capture on (`ai.capture_payloads`, opt-in) it is how long the model's whole
// request stays on disk after the work is done, and for a reading of a meeting
// transcript that request IS the transcript. 365 days is the seeded number and
// is generous for what the telemetry is for — debugging a call and auditing
// what was sent are days-to-weeks questions.
//
// It is also the only bound on the payloads an erasure cannot reach. The Art.
// 17 cascade finds them by the record a call cited and by matching the
// subject's addresses in the text; a call that names no record and whose text
// spells no address is reached by neither, and for those this window is the
// guaranteed end. config/margince.example.yaml says the same beside the switch
// that turns capture on, because that is where an operator is deciding.
func SeedDefaultRetentionTx(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO retention_policy (object_type, category, retain_days, action, lawful_basis)
		SELECT v.object_type, v.category, v.retain_days, v.action, 'storage_limitation'
		FROM (VALUES
		  ('lead',     'unconverted',        365,  'anonymize'),
		  ('activity', NULL,                 1095, 'archive'),
		  ('activity', 'transcript',         365,  'erase'),
		  ('contact',   'no_consent_no_deal', 730,  'anonymize'),
		  ('deal',     'lost',               1825, 'archive'),
		  ('ai_call_payload', 'content',     365,  'erase'),
		  ('raw_capture', NULL,              730,  'erase')
		) AS v(object_type, category, retain_days, action)`)
	return err
}
