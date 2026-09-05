// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/org360"
	"github.com/margince/margince/backend/internal/compose/orgbrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Completer is the model seam: the account-scan lane, or nil.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// Assembler is the composite read, run as the caller.
type Assembler interface {
	AssembleScoped(ctx context.Context, orgID ids.OrganizationID, opts org360.AssembleOptions) (crmcontracts.Organization360, error)
}

// Advice is the 360's own rules and the reader's dismissals, which the
// merged answer is built with rather than around.
type Advice interface {
	UndismissedAdvice(ctx context.Context, orgID ids.OrganizationID) ([]crmcontracts.Organization360Suggestion, error)
	KeepUndismissed(ctx context.Context, orgID ids.OrganizationID, found []crmcontracts.Organization360Suggestion) ([]crmcontracts.Organization360Suggestion, error)
}

// Queued is the read a row describes, as the job that runs it needs it.
type Queued struct {
	ScanID   ids.UUID
	OrgID    ids.OrganizationID
	ViewerID ids.UserID
}

// Enqueue queues the read inside the transaction that wrote its row, so the
// two either both commit or both disappear. Nil is a role with no job
// runner, which settles the floor in-request instead.
type Enqueue func(ctx context.Context, tx pgx.Tx, scan Queued) error

// RescanFloor is how soon after a settled read the same reader's next open
// may read the account again. Under it a changed account is served stale
// rather than re-read: a busy inbox must not re-read the account on every
// message.
const RescanFloor = time.Hour

// maxAdvice caps the merged list. Five is three rule rows and two of the
// model's, or the reverse; past it the card is an inventory.
const maxAdvice = 5

// Service reads, stores and serves the scan.
type Service struct {
	pool    *pgxpool.Pool
	view    Assembler
	advice  Advice
	lane    Completer
	enqueue Enqueue
	// routingVersion identifies the model binding in the fingerprint, read
	// live so a rebinding re-reads accounts rather than serving findings
	// attributed to a lane that no longer writes them.
	routingVersion func() string
	now            func() time.Time
	log            *slog.Logger
}

// NewService binds the scan to the composite read it is written from, the
// advice it is merged with, the lane that reads, and the job that runs it.
func NewService(
	pool *pgxpool.Pool, view Assembler, advice Advice, lane Completer, enqueue Enqueue,
	routingVersion func() string, now func() time.Time, log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	if routingVersion == nil {
		routingVersion = func() string { return "" }
	}
	return &Service{
		pool: pool, view: view, advice: advice, lane: lane, enqueue: enqueue,
		routingVersion: routingVersion, now: now, log: log,
	}
}

// Get is the reader's scan as it stands. Read-only: nothing here starts a read.
func (s *Service) Get(ctx context.Context, orgID ids.OrganizationID) (crmcontracts.OrganizationScan, error) {
	userID, err := s.caller(ctx)
	if err != nil {
		return crmcontracts.OrganizationScan{}, err
	}
	stored, err := s.load(ctx, userID, orgID)
	if err != nil {
		return crmcontracts.OrganizationScan{}, err
	}
	stale := false
	if stored != nil && stored.settled() && !stored.live() {
		// The fingerprint is computed only for a settled read the page will
		// hold on to: a poll of a live read wants the state, not a second
		// assembly of the account per three seconds.
		_, fingerprint, err := s.assemble(ctx, orgID)
		if err != nil {
			return crmcontracts.OrganizationScan{}, err
		}
		stale = stored.Fingerprint != nil && *stored.Fingerprint != fingerprint
	}
	return s.wire(ctx, orgID, stored, stale)
}

// Ensure makes the reader's scan current, reading the account again only
// when it moved: the stored findings when the fingerprint matches, the same
// findings marked stale when the last read is younger than the floor, and a
// queued read otherwise. A read already in flight is returned as it stands.
func (s *Service) Ensure(ctx context.Context, orgID ids.OrganizationID, force bool) (crmcontracts.OrganizationScan, error) {
	userID, err := s.caller(ctx)
	if err != nil {
		return crmcontracts.OrganizationScan{}, err
	}
	// The composite read is the gate: an account this reader may not open
	// refuses here, before any row is consulted.
	_, fingerprint, err := s.assemble(ctx, orgID)
	if err != nil {
		return crmcontracts.OrganizationScan{}, err
	}
	stored, err := s.load(ctx, userID, orgID)
	if err != nil {
		return crmcontracts.OrganizationScan{}, err
	}
	switch decide(stored, fingerprint, s.now().UTC(), force) {
	case serveCurrent:
		return s.wire(ctx, orgID, stored, false)
	case serveStale:
		return s.wire(ctx, orgID, stored, true)
	}
	queued, err := s.queue(ctx, userID, orgID)
	if err != nil {
		return crmcontracts.OrganizationScan{}, err
	}
	return s.wire(ctx, orgID, queued, false)
}

type decision int

const (
	serveCurrent decision = iota
	serveStale
	queueRead
)

// decide is the ensure rule, pure so a test can hold every branch: a live
// read is never started twice; a matching fingerprint is served; a changed
// account is read again only past the floor, unless forced. stored is nil
// for a reader who has never asked.
func decide(stored *row, fingerprint string, now time.Time, force bool) decision {
	if stored == nil {
		return queueRead
	}
	if stored.live() {
		return serveCurrent
	}
	if !stored.settled() {
		return queueRead
	}
	if !force && stored.Fingerprint != nil && *stored.Fingerprint == fingerprint {
		return serveCurrent
	}
	if !force && now.Sub(*stored.GeneratedAt) < RescanFloor {
		return serveStale
	}
	return queueRead
}

// queue writes the row and the job together, or — with no runner on this
// role — settles the floor in the same transaction, so the reader is never
// left polling a read nothing will pick up.
func (s *Service) queue(ctx context.Context, userID ids.UserID, orgID ids.OrganizationID) (*row, error) {
	var out *row
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		queued, err := queue(ctx, tx, userID, orgID)
		if err != nil {
			return err
		}
		out = &queued
		if s.enqueue == nil {
			return settle(ctx, tx, queued.ID, outcome{
				Status: StatusDegraded, GeneratedBy: crmcontracts.Deterministic,
				DegradeReason: "No worker runs account scans in this deployment, so the rules' own advice stands alone.",
			})
		}
		return s.enqueue(ctx, tx, Queued{ScanID: queued.ID, OrgID: orgID, ViewerID: userID})
	})
	if err != nil {
		return nil, err
	}
	if s.enqueue == nil {
		return s.load(ctx, userID, orgID)
	}
	return out, nil
}

// Run is the read itself, on the worker, under the viewer's own principal
// (WorkerContext). It claims the row, assembles the account as that reader,
// asks the model, and settles what grounded. A budget deferral parks the row
// and returns the typed error the job carrier snoozes on; any other failure
// closes the row as failed with a reason the reader can read.
func (s *Service) Run(ctx context.Context, scanID ids.UUID, orgID ids.OrganizationID) error {
	claimed, ok, err := s.claim(ctx, scanID)
	if err != nil || !ok {
		return err
	}
	in, fingerprint, err := s.assemble(ctx, orgID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
			return s.fail(ctx, claimed.ID, "The account could not be opened for this reader, so it was not read.")
		}
		return s.fail(ctx, claimed.ID, "The account could not be read. Try again later.")
	}
	lang := identity.BaseLanguageForPrompt(ctx, s.pool)
	findings, by, err := Read(ctx, s.lane, orgID, in, lang)
	var deferral *ai.BudgetDeferralError
	if errors.As(err, &deferral) {
		if deferErr := s.deferBudget(ctx, claimed.ID, deferral.NextAttemptAt); deferErr != nil {
			return errors.Join(err, deferErr)
		}
		return err
	}
	out := outcome{
		Status: StatusDone, Fingerprint: fingerprint, GeneratedBy: by, Findings: findings,
		ReadExchanges: len(in.Messages), ReadDeals: len(in.Account.OpenDeals),
	}
	var lane *LaneError
	switch {
	case errors.As(err, &lane):
		// The reader sees the floor and why; the cause is for whoever runs
		// the lane, and the log is where they look.
		s.log.Warn("account scan: the lane did not answer usably", "scan_id", claimed.ID, "err", lane.Cause)
		out.Status, out.DegradeReason = StatusDegraded, "The model did not answer in a form the records support, so the rules' own advice stands alone."
	case err != nil:
		// The row is claimed: left running it would be served as a read in
		// flight for as long as it sat there. It closes with the reason the
		// reader can act on, and the cause goes back to the carrier.
		return errors.Join(err, s.fail(ctx, claimed.ID, "The account could not be read. Try again later."))
	case s.lane == nil:
		out.Status, out.DegradeReason = StatusDegraded, "No model lane is configured, so the rules' own advice stands alone."
	case len(in.Messages) == 0:
		out.Status, out.DegradeReason = StatusDegraded, "There are no exchanges this reader may read, so there was nothing to read the account from."
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return settle(ctx, tx, claimed.ID, out)
	})
}

// RaisesForCaller answers the dismissal endpoint: whether the calling
// reader's stored scan carries this fingerprint. The rules' rows are the
// 360's to recognise; only the model's are asked of here.
func (s *Service) RaisesForCaller(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, fingerprint string) (bool, error) {
	userID, err := s.caller(ctx)
	if err != nil {
		return false, err
	}
	r, found, err := load(ctx, tx, userID, orgID)
	if err != nil || !found {
		return false, err
	}
	for _, finding := range r.Findings {
		if finding.Fingerprint == fingerprint {
			return true, nil
		}
	}
	return false, nil
}

// assemble reads the input as the caller: the composite read, then the
// message words, and the fingerprint over both.
func (s *Service) assemble(ctx context.Context, orgID ids.OrganizationID) (Input, string, error) {
	view, err := s.view.AssembleScoped(ctx, orgID, org360.AssembleOptions{})
	if err != nil {
		return Input{}, "", err
	}
	in := Input{Account: orgbrief.FromView(view)}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		words, err := readWords(ctx, tx, orgID, s.now().UTC())
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			// No activity grant: the account is read from its records alone,
			// and with nothing to quote the model is not asked.
			return nil
		}
		in.Messages = words
		return err
	})
	if err != nil {
		return Input{}, "", err
	}
	lang := identity.BaseLanguageForPrompt(ctx, s.pool)
	fingerprint, err := Fingerprint(in, s.routingVersion(), lang)
	if err != nil {
		return Input{}, "", err
	}
	return in, fingerprint, nil
}

// wire merges the rules' live advice with the stored findings, applies the
// reader's dismissals to the model's rows, caps, and states where the read
// stands.
func (s *Service) wire(
	ctx context.Context, orgID ids.OrganizationID, stored *row, stale bool,
) (crmcontracts.OrganizationScan, error) {
	rules, err := s.advice.UndismissedAdvice(ctx, orgID)
	if err != nil {
		return crmcontracts.OrganizationScan{}, err
	}
	var read []crmcontracts.Organization360Suggestion
	if stored != nil && len(stored.Findings) > 0 {
		read, err = s.advice.KeepUndismissed(ctx, orgID, stored.Findings)
		if err != nil {
			return crmcontracts.OrganizationScan{}, err
		}
	}
	findings, dropped := merge(rules, read)
	out := crmcontracts.OrganizationScan{
		OrganizationId:  openapi_types.UUID(orgID.UUID),
		State:           crmcontracts.OrganizationScanStateNever,
		Findings:        findings,
		FindingsDropped: dropped,
	}
	if stored == nil {
		return out, nil
	}
	r := *stored
	out.State = crmcontracts.OrganizationScanState(r.Status)
	out.GeneratedAt = r.GeneratedAt
	out.DegradeReason = r.DegradeReason
	out.ResumesAt = r.NextAttemptAt
	if r.GeneratedBy != nil {
		by := crmcontracts.WrittenBy(*r.GeneratedBy)
		out.GeneratedBy = &by
	}
	if stale {
		out.Stale = &stale
	}
	if r.ReadExchanges != nil && r.ReadDeals != nil {
		out.Read = &struct {
			Deals     int `json:"deals"`
			Exchanges int `json:"exchanges"`
		}{Deals: *r.ReadDeals, Exchanges: *r.ReadExchanges}
	}
	return out, nil
}

// merge folds both writers' advice into the list the page draws: the rules'
// rows first in their own order, then the model's in the order it gave them,
// one row per fingerprint, capped with the cap reported.
func merge(rules, read []crmcontracts.Organization360Suggestion) ([]crmcontracts.Organization360Suggestion, int) {
	seen := map[string]bool{}
	merged := make([]crmcontracts.Organization360Suggestion, 0, len(rules)+len(read))
	for _, suggestion := range append(append([]crmcontracts.Organization360Suggestion{}, rules...), read...) {
		if seen[suggestion.Fingerprint] {
			continue
		}
		seen[suggestion.Fingerprint] = true
		merged = append(merged, suggestion)
	}
	if len(merged) > maxAdvice {
		return merged[:maxAdvice], len(merged) - maxAdvice
	}
	return merged, 0
}

// caller is the human the scan belongs to. A scan is a reading aid for a
// person; an agent holding a passport has the records themselves.
func (s *Service) caller(ctx context.Context) (ids.UserID, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return ids.UserID{}, err
	}
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID == (ids.UUID{}) {
		return ids.UserID{}, fmt.Errorf("the account scan is per-user and this call carries no user: %w",
			apperrors.ErrPermissionDenied)
	}
	return ids.From[ids.UserKind](p.UserID), nil
}

// load is the reader's row, or nil for a reader who has never asked, behind
// the account's own visibility gate.
func (s *Service) load(ctx context.Context, userID ids.UserID, orgID ids.OrganizationID) (*row, error) {
	var stored *row
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "organization", orgID.UUID); err != nil {
			return err
		}
		r, found, err := load(ctx, tx, userID, orgID)
		if found {
			stored = &r
		}
		return err
	})
	return stored, err
}

func (s *Service) claim(ctx context.Context, scanID ids.UUID) (row, bool, error) {
	var r row
	var ok bool
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		r, ok, err = begin(ctx, tx, scanID)
		return err
	})
	return r, ok, err
}

func (s *Service) deferBudget(ctx context.Context, scanID ids.UUID, next time.Time) error {
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return deferBudget(ctx, tx, scanID, next)
	})
}

// fail closes a claimed row. It writes under a context that outlives the
// read's own cancellation, bounded so a dead database cannot hold the worker:
// the one failure a cancelled read must still record is that it was
// cancelled, and the row's update and the rail's announcement both ride it.
func (s *Service) fail(ctx context.Context, scanID ids.UUID, reason string) error {
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), failWriteBudget)
	defer cancel()
	return database.WithWorkspaceTx(recordCtx, s.pool, func(tx pgx.Tx) error {
		return fail(recordCtx, tx, scanID, reason)
	})
}

// failWriteBudget bounds the write that closes a failed read: long enough
// for a healthy database, short enough that a dead one does not keep the
// worker's slot.
const failWriteBudget = 5 * time.Second
