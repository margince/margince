// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

// The cache and the read around it.
//
// A cached brief is served only while its fingerprint still matches the
// account; the moment it does not, the brief is REWRITTEN before the request
// answers. So a reader is never handed text that describes a state of play
// the account has moved on from.
//
// The alternative — hand back the old brief immediately, refresh behind the
// request — trades that guarantee for a faster first paint, and needs a
// regeneration that outlives the request to do it. It is not what this does,
// and nothing in the contract claims it: a brief that arrives is current.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/briefevidence"
	"github.com/margince/margince/backend/internal/compose/company360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Assembler reads the account exactly as its reader would see it. The
// composite read is injected rather than imported so this package composes
// one seam instead of re-deriving nine gated reads.
type Assembler interface {
	AssembleScoped(ctx context.Context, companyID ids.CompanyID, opts company360.AssembleOptions) (crmcontracts.Company360, error)
}

// ProfileReader is what the company IS, as opposed to how it stands with us:
// the curated statements a site read produced and a human accepted. The 360
// does not carry them — it is the working state of the relationship — so the
// brief reads them through their own gated store method, under the same
// caller's authority.
//
// Optional: a deployment that does not wire it gets a brief about the
// relationship alone, which is what the brief was before.
type ProfileReader interface {
	ListCompanyProfileFields(ctx context.Context, companyID ids.CompanyID) ([]crmcontracts.CompanyProfileField, error)
}

// Service writes and caches the brief.
type Service struct {
	pool    *pgxpool.Pool
	view    Assembler
	profile ProfileReader
	lane    Completer
	now     func() time.Time
	// routingVersion identifies the model binding in the fingerprint, so
	// re-pointing the lane rewrites briefs rather than leaving text
	// attributed to a model that no longer writes it.
	routingVersion string
	// emailRows opens the messages the brief cites. Held rather than read
	// inline because the brief is CACHED: the sentences are one reader's to
	// keep, and the summary behind a citation is another reader's to receive,
	// so it is attached to the wire value on the way out and never to the row
	// that gets saved.
	emailRows briefevidence.Reader
}

// WithEmailSummaries binds the reader that opens a cited message.
func (s *Service) WithEmailSummaries(reader briefevidence.Reader) *Service {
	s.emailRows = reader
	return s
}

// enrich attaches the canonical email row behind every citation in a finished
// brief or answer, in one read.
//
// It takes the wire value rather than the stored one, and runs after the save,
// because a cached summary would serve the writer's own access to whoever reads
// the cache next.
func (s *Service) enrich(ctx context.Context, out crmcontracts.CompanyBrief) (crmcontracts.CompanyBrief, error) {
	var targets []briefevidence.Target
	for i := range out.Sections {
		targets = append(targets, briefevidence.FromSentences(out.Sections[i].Sentences)...)
	}
	if err := briefevidence.Attach(ctx, s.emailRows, targets); err != nil {
		return crmcontracts.CompanyBrief{}, err
	}
	return out, nil
}

// NewService binds the brief to the composite read it is written from and
// the model lane that writes it. lane may be nil: that is a deployment
// running no model, and the deterministic floor is the answer.
func NewService(pool *pgxpool.Pool, view Assembler, profile ProfileReader, lane Completer, routingVersion string, now func() time.Time) *Service {
	return &Service{pool: pool, view: view, profile: profile, lane: lane, routingVersion: routingVersion, now: now}
}

// assemble reads what a brief or an answer is written from: how the account
// stands with us, and — for the brief only — what the company is.
//
// withProfile is false for the prepared questions. None of the three answers
// from the company description, and the model never sees it either way, so
// reading it there would be one gated query per question for nothing.
//
// A profile read that fails fails the whole brief. An account with no site
// read answers with no rows, so the empty case never arrives here as an
// error — an error means the read itself broke, and treating that as "this
// company has no description" writes a brief that silently lost its second
// half AND caches it, so the next reader sees the same gap with nothing to
// say it was ever there. The reader gets one honest failure instead.
//
// projectID narrows the read to one body of work (company360.AssembleOptions);
// the scoped 360 reports what the narrowing kept, and that report is handed
// back beside the input so the wire can say so.
func (s *Service) assemble(
	ctx context.Context, companyID ids.CompanyID, withProfile bool, projectID *ids.ProjectID,
) (Input, *crmcontracts.ProjectScope, error) {
	view, err := s.view.AssembleScoped(ctx, companyID, company360.AssembleOptions{ProjectID: projectID})
	if err != nil {
		return Input{}, nil, err
	}
	in := FromView(view)
	if withProfile && s.profile != nil {
		fields, profileErr := s.profile.ListCompanyProfileFields(ctx, companyID)
		if profileErr != nil {
			return Input{}, nil, fmt.Errorf("read the company profile for the brief: %w", profileErr)
		}
		in.foldProfile(fields)
	}
	return in, view.Scope, nil
}

// Get serves the brief, regenerating when the cache no longer matches.
// force skips the cache entirely — the explicit refresh.
func (s *Service) Get(ctx context.Context, companyID ids.CompanyID, force bool) (crmcontracts.CompanyBrief, error) {
	return s.GetScoped(ctx, companyID, force, nil)
}

// GetScoped is Get narrowed to one project, when projectID is given.
//
// The cache holds ONE brief per reader and account, and the project rides
// the fingerprint rather than the row key: a scoped read after an unscoped
// one misses and rewrites, and so does the way back. That is a rewrite per
// switch, never a stale brief — a scoped brief can never be served as the
// whole account's, because their fingerprints differ.
func (s *Service) GetScoped(
	ctx context.Context, companyID ids.CompanyID, force bool, projectID *ids.ProjectID,
) (crmcontracts.CompanyBrief, error) {
	// A brief is a reading aid for a person; an agent reading records
	// through a passport has the records themselves.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.CompanyBrief{}, err
	}
	userID, err := actingUser(ctx)
	if err != nil {
		return crmcontracts.CompanyBrief{}, err
	}
	// The gates that matter run HERE, in the caller's own composite read: a
	// brief can only be written from what this caller may see, and an
	// account they cannot read refuses before any cache is consulted.
	in, scope, err := s.assemble(ctx, companyID, true, projectID)
	if err != nil {
		return crmcontracts.CompanyBrief{}, err
	}
	lang := identity.BaseLanguageForPrompt(ctx, s.pool)
	fingerprint, err := Fingerprint(in, s.routingVersion, lang)
	if err != nil {
		return crmcontracts.CompanyBrief{}, err
	}

	cached, found, err := s.cached(ctx, userID, companyID)
	if err != nil {
		return crmcontracts.CompanyBrief{}, err
	}
	// The version check is not belt-and-braces: a row written before the brief
	// had sections unmarshals cleanly into an envelope with none, and serving
	// it would render an account nobody could say anything about.
	if found && !force && cached.Version == storedVersion && cached.Fingerprint == fingerprint {
		return s.enrich(ctx, cached.wire(companyID, scope))
	}

	// The account is NAMED to the rail here, where the assembled input holds
	// the name the product shows for it everywhere else: the router sees only
	// a task and a prompt, so without this the reader's rail could say "this
	// company" and nothing more.
	sections, by, err := Write(ai.WithSubject(ctx, companyID.Ref(), in.Name), s.lane, companyID.String(), in, lang)
	if err != nil {
		return crmcontracts.CompanyBrief{}, err
	}
	written := stored{
		Fingerprint: fingerprint,
		Version:     storedVersion,
		GeneratedAt: s.now().UTC(),
		GeneratedBy: by,
		// Named AFTER grounding, whichever lane wrote the sections: a name is
		// cosmetic, read from the input the brief was written from rather
		// than trusted from the model's own reply.
		Sections: withSectionEvidenceNames(sections, in),
	}
	if err := s.save(ctx, userID, companyID, written); err != nil {
		return crmcontracts.CompanyBrief{}, err
	}
	return s.enrich(ctx, written.wire(companyID, scope))
}

// Ask answers one prepared question about the account.
//
// Nothing is cached. A brief is standing text every visit re-reads, so a cache
// earns its keep there; a question is asked once and read once, and a cached
// answer would only introduce the chance of answering from an account state
// the reader has already moved past.
func (s *Service) Ask(
	ctx context.Context, companyID ids.CompanyID, raw crmcontracts.CompanyQuestion,
) (crmcontracts.CompanyAnswer, error) {
	return s.AskScoped(ctx, companyID, raw, nil)
}

// AskScoped is Ask narrowed to one project, when projectID is given.
func (s *Service) AskScoped(
	ctx context.Context, companyID ids.CompanyID, raw crmcontracts.CompanyQuestion, projectID *ids.ProjectID,
) (crmcontracts.CompanyAnswer, error) {
	// A prepared question is a reading aid for a person; an agent asking about
	// an account has the records themselves.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.CompanyAnswer{}, err
	}
	question, err := ParseQuestion(raw)
	if err != nil {
		return crmcontracts.CompanyAnswer{}, err
	}
	// The gates that matter run HERE, in the caller's own composite read: an
	// answer can only be written from what this caller may see, and an account
	// they cannot read refuses before a single word is written.
	in, scope, err := s.assemble(ctx, companyID, false, projectID)
	if err != nil {
		return crmcontracts.CompanyAnswer{}, err
	}
	sentences, by, err := Answer(ai.WithSubject(ctx, companyID.Ref(), in.Name), s.lane, question, companyID.String(), in,
		identity.BaseLanguageForPrompt(ctx, s.pool))
	if err != nil {
		return crmcontracts.CompanyAnswer{}, err
	}
	out := crmcontracts.CompanyAnswer{
		CompanyId:   openapi_types.UUID(companyID.UUID),
		Question:    question,
		GeneratedAt: s.now().UTC(),
		GeneratedBy: by,
		Scope:       scope,
		Sentences:   wireSentences(withEvidenceNames(sentences, in)),
	}
	if err := briefevidence.Attach(ctx, s.emailRows, briefevidence.FromSentences(out.Sentences)); err != nil {
		return crmcontracts.CompanyAnswer{}, err
	}
	return out, nil
}

// storedVersion is the cached payload's shape version. A row written before
// the brief had sections unmarshals into a zero envelope, which reads as a
// cache MISS and rewrites — rather than as a brief with no sections, which
// would render as an account nobody could say anything about.
const storedVersion = 2

// stored is the cached payload's shape.
type stored struct {
	Fingerprint string                 `json:"-"`
	Version     int                    `json:"version"`
	GeneratedAt time.Time              `json:"generated_at"`
	GeneratedBy crmcontracts.WrittenBy `json:"generated_by"`
	Sections    []Section              `json:"sections"`
	// Sentences is the FLAT shape, still what Ask answers in. The brief moved
	// to sections; a question and its answer did not.
	Sentences []Sentence `json:"sentences,omitempty"`
}

// scope is the narrowing this read ran under, reported beside the text
// whether the text was written now or served from the cache: the counts are
// the account's as of THIS read, which is the only reading a scope line is
// about.
func (b stored) wire(companyID ids.CompanyID, scope *crmcontracts.ProjectScope) crmcontracts.CompanyBrief {
	sections := make([]crmcontracts.CompanyBriefSection, 0, len(b.Sections))
	for _, section := range b.Sections {
		wired := wireSentences(section.Sentences)
		if len(wired) == 0 {
			// A section whose every sentence lost its citations says nothing.
			// A heading over silence reads as a finding of nothing.
			continue
		}
		sections = append(sections, crmcontracts.CompanyBriefSection{
			Kind:      crmcontracts.CompanyBriefSectionKind(section.Kind),
			Sentences: wired,
		})
	}
	return crmcontracts.CompanyBrief{
		CompanyId:   openapi_types.UUID(companyID.UUID),
		GeneratedAt: b.GeneratedAt,
		GeneratedBy: b.GeneratedBy,
		Scope:       scope,
		Sections:    sections,
	}
}

// wireSentences renders one section's sentences, dropping the WHOLE sentence
// when any citation is not an id.
//
// Dropping only the bad citation would leave a readable claim standing on
// partial or empty evidence, and the section would still render — the reader
// sees an assertion about their account with nothing to check it against,
// which is the one thing the grounding rule exists to prevent. A sentence is
// kept only when it cited something and every citation parsed.
func wireSentences(in []Sentence) []crmcontracts.CompanyBriefSentence {
	out := make([]crmcontracts.CompanyBriefSentence, 0, len(in))
	for _, sentence := range in {
		if len(sentence.Evidence) == 0 {
			continue
		}
		evidence := make([]crmcontracts.CompanyBriefEvidence, 0, len(sentence.Evidence))
		malformed := false
		for _, cited := range sentence.Evidence {
			parsed, err := ids.Parse(cited.EntityID)
			if err != nil {
				malformed = true
				break
			}
			wired := crmcontracts.CompanyBriefEvidence{
				EntityId:   openapi_types.UUID(parsed),
				EntityType: crmcontracts.CompanyBriefEvidenceEntityType(cited.EntityType),
			}
			if cited.Name != "" {
				wired.Name = &cited.Name
			}
			evidence = append(evidence, wired)
		}
		if malformed {
			continue
		}
		wired := crmcontracts.CompanyBriefSentence{Text: sentence.Text, Evidence: evidence}
		if sentence.Nature != "" {
			nature := crmcontracts.CompanyBriefSentenceNature(sentence.Nature)
			wired.Nature = &nature
		}
		out = append(out, wired)
	}
	return out
}

// cached reads this user's brief for this account. The user_id predicate is
// the whole scope and has to be written out: without it one rep would read
// another's brief — which was written from records they may not share. It is
// also sufficient — core 0225 collapsed company_brief's unique key to
// (user_id, company_id).
func (s *Service) cached(ctx context.Context, userID ids.UserID, companyID ids.CompanyID) (stored, bool, error) {
	var out stored
	var payload []byte
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT fingerprint, generated_at, generated_by, payload FROM company_brief
			WHERE user_id = $1 AND company_id = $2`,
			userID, companyID).Scan(&out.Fingerprint, &out.GeneratedAt, &out.GeneratedBy, &payload)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{}, false, nil
	}
	if err != nil {
		return stored{}, false, err
	}
	// The WHOLE envelope, version and sections included. Unmarshalling the
	// payload into the sentence list alone left Version at zero on every read,
	// so the version check below could never pass and the cache never once
	// answered — a model call on every page load, invisible to a build gate
	// because nothing about the served brief looked wrong.
	if err := json.Unmarshal(payload, &out); err != nil {
		// A payload this build cannot read is a cache MISS, not a failure:
		// the brief is derived content, regenerating it costs one call, and
		// the new row replaces the unreadable one.
		//nolint:nilerr // an unreadable cache entry is a miss by design; the caller regenerates
		return stored{}, false, nil
	}
	return out, true, nil
}

func (s *Service) save(ctx context.Context, userID ids.UserID, companyID ids.CompanyID, brief stored) error {
	// The whole envelope, so a later read can tell which shape it is holding.
	// Storing the sentence list alone dropped both the version and the
	// sections it was meant to guard.
	payload, err := json.Marshal(brief)
	if err != nil {
		return fmt.Errorf("encode the brief payload: %w", err)
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO company_brief (user_id, company_id, fingerprint,
			                       generated_at, generated_by, payload)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, company_id) DO UPDATE
			SET fingerprint = EXCLUDED.fingerprint,
			    generated_at = EXCLUDED.generated_at,
			    generated_by = EXCLUDED.generated_by,
			    payload = EXCLUDED.payload`,
			userID, companyID, brief.Fingerprint,
			brief.GeneratedAt, brief.GeneratedBy, payload)
		return err
	})
}

// actingUser resolves the human this brief belongs to. Acknowledging that
// the brief is per-user is the whole security posture, so a principal with
// no user id has no brief rather than a shared one.
func actingUser(ctx context.Context) (ids.UserID, error) {
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID == (ids.UUID{}) {
		return ids.UserID{}, fmt.Errorf("the account brief is per-user and this call carries no user: %w",
			apperrors.ErrPermissionDenied)
	}
	return ids.From[ids.UserKind](p.UserID), nil
}
