// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// FenceVerdict is the owning domain's answer on whether a subject may be
// enriched at all.
type FenceVerdict struct {
	Allowed bool
	// Reason is the skip reason to record when Allowed is false. The domain
	// picks it because only the domain knows whether the refusal was consent
	// (suppressed) or eligibility (not_eligible).
	Reason provider.SkipReason
}

// The callbacks the owning domain supplies, declared here rather than in
// shared/ports/provider because each needs a transaction and that package is
// stdlib-only. Compose wires them from modules/people.
type (
	// FenceSubjectFunc answers consent, suppression, objection and erasure for
	// one subject, inside the queueing transaction. It runs again immediately
	// before any claim is written, because a subject can be suppressed while a
	// paid run is in flight (PI-AC-7).
	FenceSubjectFunc func(ctx context.Context, tx pgx.Tx, personID string) (FenceVerdict, error)

	// DuplicateClusterFunc returns the other records the domain believes may
	// be the same human. Empty is a legitimate answer — a domain with no
	// duplicate signal degrades the fence to the single-record rule rather
	// than blocking work.
	DuplicateClusterFunc func(ctx context.Context, tx pgx.Tx, personID string) ([]string, error)

	// SubjectIdentifiersFunc resolves the minimum set of facts that may leave
	// the installation for this subject.
	SubjectIdentifiersFunc func(ctx context.Context, tx pgx.Tx, personID string) (provider.PersonIdentifiers, error)

	// EnqueueSubmitFunc commits the submit job in the SAME transaction as the
	// run row, so a crash can never leave a run nobody will ever submit. The
	// job's args type belongs to compose — this module cannot see River.
	EnqueueSubmitFunc func(ctx context.Context, tx pgx.Tx, runID, workspaceID string) error
)

// WithDomain binds the owning domain's callbacks. Without them the service
// has no way to fence a subject or to name what may be sent about them, so
// QueueRun refuses rather than guessing.
func (s *Store) WithDomain(fence FenceSubjectFunc, cluster DuplicateClusterFunc, idents SubjectIdentifiersFunc) *Store {
	s.fence, s.cluster, s.identifiers = fence, cluster, idents
	return s
}

// WithSubjectHold binds the fence's holding form, which the hand-off asks
// before writing anything about a subject.
//
// Its own binder rather than a fourth argument to WithDomain: the two fences
// share a type, so a call site passing them in the wrong order would compile
// and would silently drop the lock — the failure being an erasure race nobody
// sees until it happens.
func (s *Store) WithSubjectHold(fn FenceSubjectFunc) *Store {
	s.holdSubject = fn
	return s
}

// WithSubmitEnqueue binds the durable hand-off to the worker.
func (s *Store) WithSubmitEnqueue(fn EnqueueSubmitFunc) *Store {
	s.enqueueSubmit = fn
	return s
}

// QueueRun admits, fences, freezes and reserves in ONE transaction, then
// returns. It never calls a provider: submission is a durable job, which is
// what lets the HTTP surface answer 202 immediately and what keeps a slow
// vendor off the request path.
//
// The order below is not arbitrary. Every step that can refuse runs before
// the step that costs money, and the fingerprint is computed before the
// duplicate check because the check is defined over it.
func (s *Store) QueueRun(ctx context.Context, in provider.QueueInput) (provider.Run, error) {
	// A run does not read a person: it BUYS facts about them and writes those
	// facts onto their record (people.WriteProviderClaims, audited as
	// update/person), spending the installation's credits on the way. So the
	// grant it asks for is the one that authorizes that write. `person` rather
	// than `integrations` deliberately: `integrations` governs the CONNECTION
	// (admin/ops configuration), while enriching someone is a thing a rep does
	// in the course of their own work, on records they may already change.
	//
	// Read was the wrong half of the pair, and read_only is what made that
	// visible — a seat whose entire purpose is that it changes nothing passed
	// this gate and left provider claims on a person.
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return provider.Run{}, err
	}
	if s.fence == nil || s.identifiers == nil {
		return provider.Run{}, errors.New("integrations: no owning domain is bound, so no subject can be fenced")
	}
	if s.enqueueSubmit == nil {
		return provider.Run{}, errors.New("integrations: no submit enqueue is bound, so a queued run would never execute")
	}
	name, err := s.resolveProvider(ctx, in.Provider)
	if err != nil {
		return provider.Run{}, err
	}
	// From here on the RESOLVED name is the provider, so every downstream
	// read (the daily ceiling, the freshness window, the duplicate cluster,
	// the run row itself) sees the one this run is actually for. An automatic
	// trigger arrives with the field empty, and leaving it that way would
	// write a run naming no provider — which the row's CHECK refuses, loudly,
	// after the fences have already passed.
	in.Provider = name
	desc, err := s.registry.Descriptor(name)
	if err != nil {
		return provider.Run{}, provider.ErrNotConnected
	}

	var out provider.Run
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The ROW gate, not just the object gate. auth.Require above answers
		// "may this role change people at all"; this answers "may this caller
		// change THIS person". Visibility is not enough and on this table it is
		// nothing at all: person is an identity table, read by every seat, so
		// the visibility arm renders TRUE for everyone and a rep could name any
		// colleague's person id, spend the installation's credits, and land
		// provider claims on a record outside their write scope.
		//
		// Live, because the run's claims are new rows on the subject and
		// archived means frozen — including a subject an Art. 17 erasure has
		// just cleared, which a run in flight would otherwise refill.
		// Existence-hiding survives: an invisible subject still answers 404,
		// and only a visible-but-not-writable one answers 403.
		if err := auth.EnsureWritableLive(ctx, tx, "person", uuidOf(&in.PersonID)); err != nil {
			return err
		}
		conn, err := s.admit(ctx, tx, name, in.Trigger)
		if err != nil {
			return err
		}
		run, err := s.queueOne(ctx, tx, desc, conn, in)
		if err != nil {
			return err
		}
		out = run
		return nil
	})
	if err != nil {
		return provider.Run{}, err
	}
	return out, nil
}

// resolveProvider answers which provider a run is for.
//
// A named one is used as given — a human clicking "enrich" on a card names
// the provider that card is about. An EMPTY name is the automatic path's
// case, and the port declares what it means: every connected provider that
// admits this trigger. In practice that is the ONE connected provider, since
// the connection table holds a row per provider and only one vendor is
// registered today; naming it here rather than making the consumer look it up
// keeps the consumer from needing to know what is registered at all.
//
// More than one connected provider is not an error to guess at: it would mean
// buying the same person's data twice, so it refuses until the platform
// declares which one an automatic trigger uses.
func (s *Store) resolveProvider(ctx context.Context, named string) (string, error) {
	if named != "" {
		return named, nil
	}
	var names []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT provider FROM provider_connection WHERE status = 'connected' ORDER BY provider`)
		if err != nil {
			return fmt.Errorf("integrations: reading the connected providers: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return fmt.Errorf("integrations: scanning a connected provider: %w", err)
			}
			names = append(names, name)
		}
		return rows.Err()
	})
	if err != nil {
		return "", err
	}
	switch len(names) {
	case 1:
		return names[0], nil
	case 0:
		// The ordinary case, and the one callers swallow: nobody connected a
		// provider, so nothing is bought and nothing is wrong.
		return "", provider.ErrNotConnected
	default:
		// Several connected, and this trigger has no rule for choosing. Its
		// OWN sentinel, because the consumer swallows ErrNotConnected: sharing
		// one would mean an installation that silently enriched nobody, for
		// ever, with no log line and no skipped run to explain it.
		return "", ErrProviderAmbiguous
	}
}

// refuseBeforeSpending answers every reason not to spend on this subject, in
// the order that reads cheapest: the standing and wallet checks first, which
// need nothing about the person, then the two that need their identifiers.
//
// It returns the skip reason to record, or "" to carry on. On the carry-on
// path it also returns the input fingerprint, computed here because the
// duplicate fence is defined over it — handing it back rather than recomputing
// it keeps one answer to "what did we ask about this subject".
//
// Separated from queueOne because these refusals share a shape: each is a fact
// about the subject's standing, the installation's wallet, or what the record
// carries, and every one must be settled before a single credit is reserved.
func (s *Store) refuseBeforeSpending(ctx context.Context, tx pgx.Tx, desc provider.Descriptor, conn admittedConnection, in provider.QueueInput, cats []provider.Category) (string, provider.SkipReason, error) {
	const none = ""
	refusal, err := s.refuseOnStanding(ctx, tx, conn, in)
	if err != nil || refusal != "" {
		return none, refusal, err
	}
	return s.refuseOnWhatTheRecordCarries(ctx, tx, desc, in, cats)
}

// refuseOnStanding answers the refusals that need nothing about the person:
// whether we may contact them at all, and whether the installation should
// spend right now.
func (s *Store) refuseOnStanding(ctx context.Context, tx pgx.Tx, conn admittedConnection, in provider.QueueInput) (provider.SkipReason, error) {
	// Consent, suppression, objection, erasure. Before anything else, because
	// a subject we may not contact must not even be looked up.
	verdict, err := s.fence(ctx, tx, in.PersonID)
	if err != nil {
		return "", err
	}
	if !verdict.Allowed {
		return verdict.Reason, nil
	}

	// The rate ceiling and the freshness window (PI-PARAM-13/14).
	if conn.dailyLimit != nil {
		spent, err := s.runsSubmittedToday(ctx, tx, in.Provider)
		if err != nil {
			return "", err
		}
		if spent >= *conn.dailyLimit {
			return provider.SkipRateLimited, nil
		}
	}
	if conn.refreshAge != nil && in.Trigger.Automatic() {
		fresh, err := s.hasFreshRun(ctx, tx, in.PersonID, in.Provider, *conn.refreshAge)
		if err != nil {
			return "", err
		}
		if fresh {
			return provider.SkipAlreadyFresh, nil
		}
	}
	return "", nil
}

// refuseOnWhatTheRecordCarries answers the two refusals that need the
// subject's identifiers, and returns the fingerprint over them.
func (s *Store) refuseOnWhatTheRecordCarries(ctx context.Context, tx pgx.Tx, desc provider.Descriptor, in provider.QueueInput, cats []provider.Category) (string, provider.SkipReason, error) {
	const none = ""
	idents, err := s.identifiers(ctx, tx, in.PersonID)
	if err != nil {
		return none, "", err
	}

	// Can this provider look the subject up at all? A record carrying neither
	// a profile link nor a name with a company gives the vendor nothing to
	// match on, and it answers with a rejection rather than an empty result.
	// The platform can only read that rejection as a provider fault — which
	// marks the whole CONNECTION broken, so one unlookupable contact makes
	// every other lookup look unavailable.
	//
	// It applies to a human's request as well as the sweep's: the button
	// cannot conjure an identifier, and a person who presses it deserves "there
	// is nothing to look up" rather than a vendor error.
	if !idents.Matchable(desc.MatchRules) {
		return none, provider.SkipNoIdentifiers, nil
	}
	fingerprint := fingerprintOf(idents, cats)

	// The duplicate fence, automatic runs only. A human asking explicitly
	// knows something the duplicate signal does not.
	if in.Trigger.Automatic() && s.cluster != nil {
		dup, err := s.duplicateAlreadyBought(ctx, tx, in, fingerprint)
		if err != nil {
			return none, "", err
		}
		if dup {
			return none, provider.SkipDuplicateSubjectCandidate, nil
		}
	}
	return fingerprint, "", nil
}

// queueOne is the admission pipeline for one subject. Each refusal writes a
// skipped run rather than nothing at all: a customer asking "why was this
// person not enriched" deserves a row that answers, and a silent no-op cannot.
func (s *Store) queueOne(ctx context.Context, tx pgx.Tx, desc provider.Descriptor, conn admittedConnection, in provider.QueueInput) (provider.Run, error) {
	snapshot := freezeSnapshot(conn)
	cats, err := runCategories(desc, conn, in)
	if err != nil {
		return provider.Run{}, err
	}

	// 1-4. Every reason not to spend, in one pass. The fingerprint comes back
	//      with the verdict because the duplicate fence is defined over it and
	//      step 5 writes it onto the run.
	fingerprint, refusal, err := s.refuseBeforeSpending(ctx, tx, desc, conn, in, cats)
	if err != nil {
		return provider.Run{}, err
	}
	if refusal != "" {
		return s.insertSkipped(ctx, tx, conn, in, snapshot, cats, refusal)
	}

	// 5. The run row, under the live-run index. A duplicate trigger for the
	//    same subject and inputs returns the run already in flight instead of
	//    buying the same answer twice.
	runID, existing, err := s.insertRun(ctx, tx, conn, in, snapshot, cats, fingerprint)
	if err != nil {
		return provider.Run{}, err
	}
	if existing {
		return s.readRun(ctx, tx, runID)
	}

	// 6. The reservation: the whole worst case, up front, all pools or none.
	skip, err := s.reserve(ctx, tx, desc, conn, runID, cats)
	if err != nil {
		return provider.Run{}, err
	}
	if skip != "" {
		if err := s.markSkipped(ctx, tx, runID, skip); err != nil {
			return provider.Run{}, err
		}
		return s.readRun(ctx, tx, runID)
	}

	// 7. The durable hand-off, committed with the run. It is REQUIRED, not
	//    optional: a queued run with no job is not a run. It would sit in the
	//    live-run index forever, blocking every later attempt at the same
	//    subject while nothing ever executed it — the failure capture's
	//    StartBackfill documents for exactly the same shape. A missing
	//    enqueue is a wiring bug, so it fails here rather than committing a
	//    row that looks queued and is inert.
	ws, err := s.db.Workspace(ctx)
	if err != nil {
		return provider.Run{}, fmt.Errorf("integrations: resolving the workspace for the submit job: %w", err)
	}
	if err := s.enqueueSubmit(ctx, tx, runID, ws.String()); err != nil {
		return provider.Run{}, fmt.Errorf("integrations: scheduling the submission: %w", err)
	}
	// "create", not "queue": the audit vocabulary is closed (0018), and
	// queueing a run IS the creation of the run row.
	if _, err := storekit.Audit(ctx, tx, "create", "provider_run", uuidOf(&runID),
		nil, map[string]any{auditKeyProvider: in.Provider, "trigger": string(in.Trigger)}); err != nil {
		return provider.Run{}, err
	}
	return s.readRun(ctx, tx, runID)
}

// runsSubmittedToday counts what actually reached the provider today. A run
// that never left `queued` cost nothing and must not consume the ceiling.
func (s *Store) runsSubmittedToday(ctx context.Context, tx pgx.Tx, name string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT count(*) FROM provider_run
		 WHERE provider = $1
		   AND state <> 'queued' AND state <> 'skipped' AND state <> 'cancelled'
		   AND created_at >= date_trunc('day', now() AT TIME ZONE 'UTC')`, name).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("integrations: counting today's runs: %w", err)
	}
	return n, nil
}

// hasFreshRun reports whether this subject's newest completed run is still
// inside the refresh window.
func (s *Store) hasFreshRun(ctx context.Context, tx pgx.Tx, personID, name string, days int) (bool, error) {
	var fresh bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM provider_run
		   WHERE person_id = $1 AND provider = $2 AND state = 'completed'
		     AND completed_at > now() - make_interval(days => $3))`,
		personID, name, days).Scan(&fresh)
	if err != nil {
		return false, fmt.Errorf("integrations: checking result freshness: %w", err)
	}
	return fresh, nil
}

// duplicateAlreadyBought asks whether any record the domain considers the same
// human already holds a completed or live run at this fingerprint.
//
// The advisory lock serializes two automatic runs racing across a duplicate
// pair: without it both would look, both would see nothing, and both would
// buy. It is keyed on the cluster's stable minimum id so both racers hash to
// the same lock whichever side they started from.
func (s *Store) duplicateAlreadyBought(ctx context.Context, tx pgx.Tx, in provider.QueueInput, fingerprint string) (bool, error) {
	cluster, err := s.cluster(ctx, tx, in.PersonID)
	if err != nil {
		return false, err
	}
	if len(cluster) == 0 {
		return false, nil
	}
	key := append([]string{in.PersonID}, cluster...)
	sort.Strings(key)
	if err := storekit.LockWriteIdentity(ctx, tx, "provider_run_cluster", key[0]); err != nil {
		return false, err
	}

	var bought bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM provider_run
		   WHERE person_id = ANY($1) AND provider = $2 AND input_fingerprint = $3
		     AND state IN ('completed','queued','submitting','in_progress','submission_unknown'))`,
		cluster, in.Provider, fingerprint).Scan(&bought)
	if err != nil {
		return false, fmt.Errorf("integrations: checking the duplicate cluster: %w", err)
	}
	return bought, nil
}

// freezeSnapshot captures what this run is allowed to do, so a later settings
// change cannot widen it (PI-AC-2).
// The snapshot records the POLICY that admitted this run, not what the run
// went on to ask for. The two differ now that a run can narrow: an automatic
// run takes only the free categories and a button buys one, while the policy
// says what the connection permitted at that moment. requested_categories on
// the run row is the second question, and a reader reconciling a charge needs
// both — what was allowed, and what was actually bought.
func freezeSnapshot(c admittedConnection) provider.Snapshot {
	return provider.Snapshot{
		Mode:             c.mode,
		Categories:       categoriesFrom(c.categories),
		AutomaticCreate:  c.autoCreate,
		AutomaticImport:  c.autoImport,
		RefreshAfterDays: c.refreshAge,
		DailyRunLimit:    c.dailyLimit,
	}
}

// fingerprintOf hashes exactly what will be SENT plus what was asked for, so a
// repeat of the SAME request finds the run already in flight rather than
// buying the same answer twice.
//
// It does not catch two runs whose category sets overlap without matching:
// they hash differently, both pass the live-run index, and both buy the
// category they share. Guarding that needs a per-category claim rather than a
// whole-set hash, which is a schema change — tracked as its own work, and said
// here rather than left for the next reader to discover from a duplicate
// charge.
func fingerprintOf(id provider.PersonIdentifiers, cats []provider.Category) string {
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		names = append(names, string(c))
	}
	sort.Strings(names)
	payload := struct {
		ID   provider.PersonIdentifiers `json:"identifiers"`
		Cats []string                   `json:"categories"`
	}{ID: id, Cats: names}
	raw, err := json.Marshal(payload)
	if err != nil {
		// Marshalling two plain structs cannot fail. If it ever did, a
		// fingerprint nothing else can equal is safer than one everything
		// matches: it would queue a fresh run rather than silently reuse one.
		raw = []byte(id.LinkedInURL + id.FirstName + id.LastName + time.Now().String())
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
