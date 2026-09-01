// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The company view's three grounded-prose lanes, and only those.
//
// They are together because they share one decision: what a reader sees when a
// model is not available. All three degrade to a deterministic floor rather
// than failing, and `generated_by` names which wrote what — but the floors are
// not equivalent. The brief and the dossier still describe something from the
// same records; growth fit's can only abstain, because grading is not a
// restatement of recorded values. That difference is why they are three options
// and not one.
//
// The dossier and the growth fit also share one handler set, so each of their
// options rebuilds it from BOTH services. Either may run first; neither may
// rebuild that set from a service it did not just construct.

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/meetingbrief"
	"github.com/margince/margince/backend/internal/compose/orgbrief"
	"github.com/margince/margince/backend/internal/compose/orgdossier"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// WithAccountDraft binds the lane that writes an account-started email
// (ADR-0087/A132) — the drafting half of the pair whose sending half is
// POST /emails.
//
// Without it the endpoint still answers, from the deterministic floor: a
// deployment running no model still has a rep who pressed "Write email", and
// a short opener they edit beats a refusal.
//
// The pool it takes is for READS only — the sender's own voice profile and the
// identity behind the envelope. Drafting still writes nothing; accountdraft is
// handed the voice READ seam rather than the store, so the zero-write
// guarantee stays a dependency rather than a rule somebody remembers.
func WithAccountDraft(brain completer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		svc := accountdraft.NewService(s.org360Svc, brain).
			WithEnvelope(draftEnvelope(pool, s.log)).
			WithDossier(s.orgDossierSvc).
			WithVoice(ai.NewVoiceStore(InstallationDB(pool)), s.log)
		s.accountDraftHandlers = accountdraft.NewHandlers(svc, s.sorDispatch.isOverlay)
	}
}

// WithAccountBrief binds the summarize lane both of the company view's
// grounded-prose surfaces are written by — the standing brief and the
// prepared "Ask Margince" questions — and the routing version that
// identifies the binding in every cached brief's fingerprint.
//
// Without it both serve their deterministic floor rather than failing: a
// role that runs no model still answers the endpoints, and generated_by
// tells the reader which of the two they have. routingVersion rides the
// fingerprint so re-pointing this lane rewrites cached briefs instead of
// leaving text attributed to a model that no longer writes it.
func WithAccountBrief(brain completer, routingVersion string) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.orgBriefSvc = orgbrief.NewService(pool, s.org360Svc, s.peopleStore, brain, routingVersion, time.Now)
		s.orgBriefHandlers = orgbrief.NewHandlers(s.orgBriefSvc, s.sorDispatch.isOverlay)
	}
}

// WithCompanyDossier binds the lane that writes what a company IS, and the
// routing version that identifies the binding in every cached dossier's
// fingerprint.
//
// Unlike the growth fit's, this lane is an improvement rather than a
// precondition: the floor already describes the company from the same fields,
// one restated value per sentence. What the model adds is prose a person reads
// before a call instead of a list they skim.
//
// It rebuilds the shared handler set from BOTH services for the reason
// WithGrowthFit does — replacing that set while remembering only one service
// would leave the other endpoint answering from a service the Server no longer
// holds. Either option may run first.
func WithCompanyDossier(brain completer, routingVersion string) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.orgDossierSvc = orgdossier.NewService(pool, s.peopleStore, brain, routingVersion, time.Now)
		s.orgDossierHandlers = orgdossier.NewHandlers(
			s.orgDossierSvc, s.orgGrowthFitSvc, s.sorDispatch.isOverlay)
	}
}

// WithGrowthFit binds the lane that judges how well a company fits what we
// sell, and the routing version that identifies the binding in every cached
// assessment's fingerprint.
//
// This is the one company-view lane whose absence CHANGES THE ANSWER rather
// than only its prose. The dossier's floor still describes a company; growth
// fit's floor abstains by DOSS-PARAM-7, because grading is not a restatement of
// recorded values. So an unwired deployment serves "here is what I would need
// to know", labelled deterministic, and never a band nobody stands behind.
//
// The dossier service is rebound alongside it rather than rebuilt, because the
// two share one handler set: replacing that set while remembering only one
// service would leave the other endpoint answering from a service the Server no
// longer holds.
func WithGrowthFit(brain completer, routingVersion string) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.orgGrowthFitSvc = orgdossier.NewGrowthFitService(
			pool, s.peopleStore, offeringConfirmed(s.peopleStore), brain, routingVersion, time.Now)
		s.orgDossierHandlers = orgdossier.NewHandlers(
			s.orgDossierSvc, s.orgGrowthFitSvc, s.sorDispatch.isOverlay)
	}
}

// WithMeetingBriefWriter binds the summarize lane that rewrites a pre-meeting
// brief in Margince's own voice.
//
// Without it the brief serves its deterministic floor rather than failing: a
// role that runs no model still answers the endpoint, and generated_by tells
// the reader which of the two they have. There is no routing version to carry
// here, unlike the account brief's: the meeting brief caches nothing, so no
// stored text can outlive a re-pointed lane.
func WithMeetingBriefWriter(brain completer) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		if s.meetingBriefSvc == nil {
			return
		}
		s.meetingBriefHandlers = meetingbrief.NewHandlers(
			s.meetingBriefSvc.WithLane(brain), s.sorDispatch.isOverlay)
	}
}

// WithDealStatusWriter binds the deal_health lane that writes the deal status
// card's words.
//
// Without it the card is its deterministic composition rather than a failure:
// a role that runs no model still answers the endpoint, and generated_by tells
// the reader which writer they have.
//
// The routing version rides along because the card IS cached: a card written
// under one routing configuration must not be served after the configuration
// changes, so the version is part of the fingerprint.
func WithDealStatusWriter(brain completer, routingVersion string) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		if s.dealStatusSvc == nil {
			return
		}
		s.dealStatusHandlers = dealstatus.NewHandlers(s.dealStatusSvc.WithLane(brain, routingVersion))
	}
}

// WithRoleProposals binds the lane that reads a deal's buying roles out of
// what its contacts have written.
//
// Unlike every other lane on this page, its absence is a 501 rather than a
// deterministic answer. Each of the others degrades to a template — a draft
// still has words, a status card still has the deal's own facts. A buying role
// has no template, because the only thing left to read one from would be the
// job title, and inferring a role from a title is precisely what this endpoint
// exists not to do. So a role that wires no lane declares the capability
// absent instead of guessing.
func WithRoleProposals(brain completer) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		if s.org360Svc == nil {
			return
		}
		s.org360Handlers = s.WithRoleLane(brain)
	}
}

// WithIntroRequestDraft binds the lane that phrases an introduction request.
//
// Its absence is a template, not a 501 — the opposite of WithRoleProposals
// beside it, and for a reason worth stating: an ask for an introduction is
// short, and its facts are few enough that a template states every one of them
// honestly. What the model buys here is phrasing. A buying role has no such
// floor, because the only thing left to read one from is the job title.
func WithIntroRequestDraft(brain completer) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		if s.org360Svc == nil {
			return
		}
		s.org360Handlers = s.WithIntroLane(brain)
	}
}

// WithIntroNoteDraft binds the lane that phrases the forwardable note.
//
// The OTHER message in one workflow. WithIntroRequestDraft above binds the ask
// written TO a colleague; this binds the prospect-facing note that colleague
// forwards. Two options rather than one because they are two prompts with two
// registers, and a deployment could reasonably want the internal one and not
// the outward one — the note is read by a customer.
//
// Its absence is a template, not a 501, for the reason stated above.
func WithIntroNoteDraft(brain completer) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		s.Reads = s.WithIntroNoteLane(brain)
	}
}
