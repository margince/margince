// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The person view's model-backed lanes.
//
// Separate from the company view's options because the two views degrade
// independently: a deployment that lights up one has said nothing about the
// other, and folding them into one option would tie a person-page decision to a
// company-page one.

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/persondraft"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// WithPersonDraft binds the lane that writes an email to one contact — the
// person-side mirror of WithAccountDraft, whose sending half is likewise
// POST /emails.
//
// Without it the endpoint still answers, from the deterministic floor: a
// deployment running no model still has a rep who pressed "Write email", and a
// short opener they edit beats a refusal.
//
// The pool it takes is for READS only — the sender's own voice profile and the
// identity behind the envelope. Drafting still writes nothing; persondraft is
// handed the voice READ seam rather than the store, so the zero-write
// guarantee stays a dependency rather than a rule somebody remembers.
func WithPersonDraft(brain completer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		svc := persondraft.NewService(s.person360Svc, brain).
			WithEnvelope(draftEnvelope(pool, s.log)).
			WithVoice(ai.NewVoiceStore(InstallationDB(pool)), s.log)
		s.personDraftHandlers = persondraft.NewHandlers(svc, s.sorDispatch.isOverlay)
	}
}
